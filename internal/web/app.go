package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"debate/internal/orchestrator"
	"debate/internal/persona"
)

const (
	defaultAddr       = ":8080"
	maxRequestBytes   = 2 * 1024 * 1024
	serverStopTimeout = 5 * time.Second
	defaultRunTimeout = 30 * time.Minute
	defaultTurnBuffer = 600
)

type Runner interface {
	Run(ctx context.Context, problem string, personas []persona.Persona, onTurn func(orchestrator.Turn)) (orchestrator.Result, error)
}

type ConfigurableRunner interface {
	RunWithConfig(ctx context.Context, problem string, personas []persona.Persona, cfg orchestrator.Config, onTurn func(orchestrator.Turn)) (orchestrator.Result, error)
}

type LoaderFunc func(path string) ([]persona.Persona, error)

type Config struct {
	PersonaPath string
	BaseDir     string
	OutputDir   string
	Runner      Runner
	// RunnerDefaults is the baseline config used when per-request runtime
	// tuning overrides are provided.
	RunnerDefaults orchestrator.Config
	Loader         LoaderFunc
	Now            func() time.Time
	RunTimeout     time.Duration
	TurnBuffer     int
}

type App struct {
	personaPath string
	baseDir     string
	outputDir   string
	runner      Runner
	runnerCfg   orchestrator.Config
	loader      LoaderFunc
	now         func() time.Time
	runTimeout  time.Duration
	turnBuffer  int
	runsMu      sync.RWMutex
	runs        map[string]*debateRun
	runSeq      uint64
	outputSeq   uint64
}

type debateRequest struct {
	Problem                 string            `json:"problem"`
	PersonaPath             string            `json:"persona_path,omitempty"`
	Personas                []persona.Persona `json:"personas,omitempty"`
	AudienceMode            *string           `json:"audience_mode,omitempty"`
	MaxTurns                *int              `json:"max_turns,omitempty"`
	ConsensusThreshold      *float64          `json:"consensus_threshold,omitempty"`
	MaxNoProgressJudges     *int              `json:"max_no_progress_judges,omitempty"`
	NoProgressEpsilon       *float64          `json:"no_progress_epsilon,omitempty"`
	UnlimitedHardMaxTurns   *int              `json:"unlimited_hard_max_turns,omitempty"`
	DirectHandoffJudgeEvery *int              `json:"direct_handoff_judge_every,omitempty"`
	LLMHistoryTurnWindow    *int              `json:"llm_history_turn_window,omitempty"`
	MaxDurationSeconds      *int              `json:"max_duration_seconds,omitempty"`
	MaxTotalTokens          *int              `json:"max_total_tokens,omitempty"`
	RunTimeoutSeconds       *int              `json:"run_timeout_seconds,omitempty"`
}

type debateResponse struct {
	Result            orchestrator.Result `json:"result"`
	SavedJSONPath     string              `json:"saved_json_path"`
	SavedMarkdownPath string              `json:"saved_markdown_path"`
}

type personasResponse struct {
	Path     string            `json:"path"`
	Personas []persona.Persona `json:"personas"`
}

type streamStartEvent struct {
	Problem      string `json:"problem"`
	PersonaPath  string `json:"persona_path,omitempty"`
	PersonaCount int    `json:"persona_count"`
}

type streamStartResponse struct {
	RunID        string `json:"run_id"`
	Problem      string `json:"problem"`
	PersonaPath  string `json:"persona_path,omitempty"`
	PersonaCount int    `json:"persona_count"`
}

type streamStopRequest struct {
	RunID string `json:"run_id"`
}

type streamStopResponse struct {
	RunID  string `json:"run_id"`
	Status string `json:"status"`
}

type streamStoppedEvent struct {
	RunID  string `json:"run_id"`
	Status string `json:"status"`
}

func NewApp(cfg Config) *App {
	if cfg.Loader == nil {
		cfg.Loader = persona.LoadFromFile
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.RunTimeout <= 0 {
		cfg.RunTimeout = defaultRunTimeout
	}
	if cfg.TurnBuffer <= 0 {
		cfg.TurnBuffer = defaultTurnBuffer
	}
	baseDir := strings.TrimSpace(cfg.BaseDir)
	if baseDir == "" {
		wd, err := os.Getwd()
		if err == nil {
			baseDir = wd
		} else {
			baseDir = "."
		}
	}
	if abs, err := filepath.Abs(baseDir); err == nil {
		baseDir = abs
	}

	return &App{
		personaPath: cfg.PersonaPath,
		baseDir:     filepath.Clean(baseDir),
		outputDir:   cfg.OutputDir,
		runner:      cfg.Runner,
		runnerCfg:   cfg.RunnerDefaults,
		loader:      cfg.Loader,
		now:         cfg.Now,
		runTimeout:  cfg.RunTimeout,
		turnBuffer:  cfg.TurnBuffer,
		runs:        make(map[string]*debateRun),
	}
}

func (a *App) Start(ctx context.Context, addr string) error {
	if a.runner == nil {
		return errors.New("runner is required")
	}
	if strings.TrimSpace(addr) == "" {
		addr = defaultAddr
	}

	server := &http.Server{
		Addr:    addr,
		Handler: a.Handler(),
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), serverStopTimeout)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.HandleFunc("/", a.handleIndex)
	mux.HandleFunc("/api/personas", a.handlePersonas)
	mux.HandleFunc("/api/debate", a.handleDebate)
	mux.HandleFunc("/api/debate/stream/start", a.handleDebateStreamStart)
	mux.HandleFunc("/api/debate/stream", a.handleDebateStream)
	mux.HandleFunc("/api/debate/stream/stop", a.handleDebateStreamStop)
	return mux
}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, indexHTML)
}

func (a *App) handlePersonas(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	loaderPath, displayPath, err := a.resolvePersonaPath(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("resolve personas path: %v", err))
		return
	}
	personas, err := a.loader(loaderPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("load personas: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, personasResponse{
		Path:     displayPath,
		Personas: personas,
	})
}

func (a *App) handleDebate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	body := http.MaxBytesReader(w, r.Body, maxRequestBytes)
	defer body.Close()

	req, err := decodeDebateRequest(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	personas, _, err := a.resolvePersonas(req.PersonaPath, req.Personas)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("load personas: %v", err))
		return
	}

	runCfg, err := a.resolveRunnerConfig(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	runCtx, cancel, err := a.contextWithRuntimeTimeout(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if cancel != nil {
		defer cancel()
	}

	resp, err := a.runAndSaveDebate(runCtx, req.Problem, personas, runCfg, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (a *App) resolveRunnerConfig(req debateRequest) (*orchestrator.Config, error) {
	if !req.hasRunnerTuning() {
		return nil, nil
	}
	if _, ok := a.runner.(ConfigurableRunner); !ok {
		return nil, errors.New("runtime tuning is not supported by the current runner")
	}
	cfg := req.applyRunnerTuning(a.runnerCfg)
	return &cfg, nil
}
