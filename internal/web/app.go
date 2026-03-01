package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"debate/internal/orchestrator"
	"debate/internal/output"
	"debate/internal/persona"
)

const (
	defaultAddr       = ":8080"
	maxRequestBytes   = 2 * 1024 * 1024
	serverStopTimeout = 5 * time.Second
	maxPathCollisions = 1000
)

type Runner interface {
	Run(ctx context.Context, problem string, personas []persona.Persona, onTurn func(orchestrator.Turn)) (orchestrator.Result, error)
}

type LoaderFunc func(path string) ([]persona.Persona, error)

type Config struct {
	PersonaPath string
	BaseDir     string
	OutputDir   string
	Runner      Runner
	Loader      LoaderFunc
	Now         func() time.Time
}

type App struct {
	personaPath string
	baseDir     string
	outputDir   string
	runner      Runner
	loader      LoaderFunc
	now         func() time.Time
	runsMu      sync.RWMutex
	runs        map[string]*debateRun
	runSeq      uint64
}

type debateRequest struct {
	Problem     string            `json:"problem"`
	PersonaPath string            `json:"persona_path,omitempty"`
	Personas    []persona.Persona `json:"personas,omitempty"`
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
		loader:      cfg.Loader,
		now:         cfg.Now,
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

	resp, err := a.runAndSaveDebate(r.Context(), req.Problem, personas, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (a *App) handleDebateStreamStart(w http.ResponseWriter, r *http.Request) {
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

	personas, resolvedPath, err := a.resolvePersonas(req.PersonaPath, req.Personas)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("load personas: %v", err))
		return
	}

	runID := a.nextRunID()
	runCtx, cancel := context.WithCancel(context.Background())
	run := newDebateRun(runID, streamStartEvent{
		Problem:      req.Problem,
		PersonaPath:  resolvedPath,
		PersonaCount: len(personas),
	}, cancel)
	a.storeRun(run)

	go a.executeDebateRun(runCtx, runID, run, req.Problem, personas)

	writeJSON(w, http.StatusAccepted, streamStartResponse{
		RunID:        runID,
		Problem:      req.Problem,
		PersonaPath:  resolvedPath,
		PersonaCount: len(personas),
	})
}

func (a *App) handleDebateStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming is not supported by this server")
		return
	}

	runID := strings.TrimSpace(r.URL.Query().Get("run_id"))
	if runID == "" {
		writeError(w, http.StatusBadRequest, "run_id is required")
		return
	}

	run, ok := a.loadRun(runID)
	if !ok {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	if err := writeSSE(w, flusher, "start", run.start); err != nil {
		return
	}

	cursor := 0
	for {
		newTurns, done, stopped, resp, runErr := run.snapshot(cursor)
		for _, turn := range newTurns {
			if err := writeSSE(w, flusher, "turn", turn); err != nil {
				return
			}
			cursor++
		}

		if done {
			if stopped {
				_ = writeSSE(w, flusher, "stopped", streamStoppedEvent{
					RunID:  runID,
					Status: "stopped",
				})
				return
			}
			if runErr != nil {
				_ = writeSSE(w, flusher, "debate_error", map[string]string{
					"error": runErr.Error(),
				})
				return
			}
			_ = writeSSE(w, flusher, "complete", resp)
			return
		}

		if err := run.waitForUpdate(r.Context()); err != nil {
			return
		}
	}
}

func (a *App) handleDebateStreamStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	body := http.MaxBytesReader(w, r.Body, maxRequestBytes)
	defer body.Close()

	req, err := decodeStreamStopRequest(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	run, ok := a.loadRun(req.RunID)
	if !ok {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}

	run.stop()
	writeJSON(w, http.StatusOK, streamStopResponse{
		RunID:  req.RunID,
		Status: "stopping",
	})
}

func (a *App) executeDebateRun(ctx context.Context, runID string, run *debateRun, problem string, personas []persona.Persona) {
	resp, err := a.runAndSaveDebate(ctx, problem, personas, run.appendTurn)
	run.finish(resp, err)
	time.AfterFunc(runRetention, func() {
		a.deleteRun(runID)
	})
}

func (a *App) nextRunID() string {
	seq := atomic.AddUint64(&a.runSeq, 1)
	return fmt.Sprintf("run-%s-%06d", a.now().UTC().Format("20060102-150405.000000000"), seq)
}

func (a *App) storeRun(run *debateRun) {
	a.runsMu.Lock()
	defer a.runsMu.Unlock()
	a.runs[run.id] = run
}

func (a *App) loadRun(runID string) (*debateRun, bool) {
	a.runsMu.RLock()
	defer a.runsMu.RUnlock()
	run, ok := a.runs[runID]
	return run, ok
}

func (a *App) deleteRun(runID string) {
	a.runsMu.Lock()
	defer a.runsMu.Unlock()
	delete(a.runs, runID)
}

func (a *App) resolvePersonas(personaPath string, inline []persona.Persona) ([]persona.Persona, string, error) {
	if len(inline) > 0 && strings.TrimSpace(personaPath) != "" {
		return nil, "", errors.New("persona_path and personas cannot be used together")
	}
	if len(inline) > 0 {
		normalized, err := persona.NormalizeAndValidate(inline)
		if err != nil {
			return nil, "", err
		}
		return normalized, "", nil
	}

	loaderPath, displayPath, err := a.resolvePersonaPath(personaPath)
	if err != nil {
		return nil, "", err
	}
	personas, err := a.loader(loaderPath)
	if err != nil {
		return nil, displayPath, err
	}
	normalized, err := persona.NormalizeAndValidate(personas)
	if err != nil {
		return nil, displayPath, err
	}
	return normalized, displayPath, nil
}

func (a *App) resolvePersonaPath(rawPath string) (loaderPath string, displayPath string, err error) {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		path = strings.TrimSpace(a.personaPath)
	}
	if path == "" {
		return "", "", errors.New("persona path is required")
	}
	if !strings.EqualFold(filepath.Ext(path), ".json") {
		return "", "", errors.New("persona path must be a .json file")
	}

	cleanPath := filepath.Clean(path)
	candidateAbs := cleanPath
	if !filepath.IsAbs(candidateAbs) {
		candidateAbs = filepath.Join(a.baseDir, cleanPath)
	}
	candidateAbs, err = filepath.Abs(candidateAbs)
	if err != nil {
		return "", "", fmt.Errorf("abs path: %w", err)
	}

	baseForCheck, err := resolvePathForContainment(a.baseDir)
	if err != nil {
		return "", "", fmt.Errorf("resolve base path: %w", err)
	}
	candidateForCheck, err := resolvePathForContainment(candidateAbs)
	if err != nil {
		return "", "", fmt.Errorf("resolve persona path: %w", err)
	}
	isWithinBase, err := pathWithinBase(baseForCheck, candidateForCheck)
	if err != nil {
		return "", "", fmt.Errorf("relative path: %w", err)
	}
	if !isWithinBase {
		return "", "", errors.New("persona path must stay within the project directory")
	}

	relToBase, err := filepath.Rel(a.baseDir, candidateAbs)
	if err != nil {
		return "", "", fmt.Errorf("loader relative path: %w", err)
	}
	if relToBase == ".." || strings.HasPrefix(relToBase, ".."+string(filepath.Separator)) {
		return "", "", errors.New("persona path must stay within the project directory")
	}
	relToBase = filepath.Clean(relToBase)
	displayPath = filepath.ToSlash(relToBase)
	if !strings.HasPrefix(displayPath, ".") {
		displayPath = "." + string(filepath.Separator) + displayPath
		displayPath = filepath.ToSlash(displayPath)
	}
	loaderPath = candidateAbs
	return loaderPath, displayPath, nil
}

func (a *App) runAndSaveDebate(ctx context.Context, problem string, personas []persona.Persona, onTurn func(orchestrator.Turn)) (debateResponse, error) {
	result, err := a.runner.Run(ctx, problem, personas, onTurn)
	if err != nil {
		return debateResponse{}, fmt.Errorf("run debate: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return debateResponse{}, fmt.Errorf("debate canceled before save: %w", err)
	}

	savePath, err := a.nextOutputPath()
	if err != nil {
		return debateResponse{}, fmt.Errorf("prepare output path: %w", err)
	}
	if err := output.SaveResult(savePath, result); err != nil {
		return debateResponse{}, fmt.Errorf("save result: %w", err)
	}

	return debateResponse{
		Result:            result,
		SavedJSONPath:     savePath,
		SavedMarkdownPath: output.MarkdownPath(savePath),
	}, nil
}

func (a *App) nextOutputPath() (string, error) {
	basePath := output.NewTimestampPath(a.outputDir, a.now())
	available, err := pathAvailable(basePath)
	if err != nil {
		return "", err
	}
	if available {
		return basePath, nil
	}

	ext := filepath.Ext(basePath)
	stem := strings.TrimSuffix(basePath, ext)
	for i := 1; i <= maxPathCollisions; i++ {
		candidate := fmt.Sprintf("%s-%03d%s", stem, i, ext)
		available, err := pathAvailable(candidate)
		if err != nil {
			return "", err
		}
		if available {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("failed to allocate unique output path after %d attempts", maxPathCollisions)
}

func pathAvailable(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return false, nil
	}
	if os.IsNotExist(err) {
		return true, nil
	}
	return false, fmt.Errorf("stat output path %q: %w", path, err)
}

func decodeDebateRequest(body io.Reader) (debateRequest, error) {
	var req debateRequest
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return debateRequest{}, fmt.Errorf("invalid request body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return debateRequest{}, errors.New("invalid request body: multiple json values are not allowed")
	}

	req.Problem = strings.TrimSpace(req.Problem)
	if req.Problem == "" {
		return debateRequest{}, errors.New("problem is required")
	}
	return req, nil
}

func decodeStreamStopRequest(body io.Reader) (streamStopRequest, error) {
	var req streamStopRequest
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return streamStopRequest{}, fmt.Errorf("invalid request body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return streamStopRequest{}, errors.New("invalid request body: multiple json values are not allowed")
	}
	req.RunID = strings.TrimSpace(req.RunID)
	if req.RunID == "" {
		return streamStopRequest{}, errors.New("run_id is required")
	}
	return req, nil
}

func resolvePathForContainment(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	switch {
	case err == nil:
		path = resolved
	case os.IsNotExist(err):
		// Keep original path for non-existent targets.
	default:
		return "", fmt.Errorf("evaluate symlink: %w", err)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absPath), nil
}

func pathWithinBase(baseAbs, candidateAbs string) (bool, error) {
	relToBase, err := filepath.Rel(baseAbs, candidateAbs)
	if err != nil {
		return false, err
	}
	if relToBase == ".." || strings.HasPrefix(relToBase, ".."+string(filepath.Separator)) {
		return false, nil
	}
	return true, nil
}

func writeSSE(w io.Writer, flusher http.Flusher, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func methodNotAllowed(w http.ResponseWriter, allowed string) {
	w.Header().Set("Allow", allowed)
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
