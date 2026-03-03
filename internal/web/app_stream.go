package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"debate/internal/orchestrator"
	"debate/internal/persona"
)

const maxTimerDuration = time.Duration(1<<63 - 1)

func timeoutWithRetention(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return runRetention
	}
	if timeout > maxTimerDuration-runRetention {
		return maxTimerDuration
	}
	return timeout + runRetention
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

	runCfg, err := a.resolveRunnerConfig(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	timeout := a.runTimeout
	if req.RunTimeoutSeconds != nil {
		timeout = req.runTimeoutDuration()
	}

	runID := a.nextRunID()
	runCtx, cancel := context.WithTimeout(context.Background(), timeout)
	run := newDebateRun(runID, streamStartEvent{
		Problem:      req.Problem,
		PersonaPath:  resolvedPath,
		PersonaCount: len(personas),
	}, cancel, a.turnBuffer)
	a.storeRun(run)
	time.AfterFunc(timeoutWithRetention(timeout), func() {
		run.stop()
		a.deleteRun(runID)
	})

	go a.executeDebateRun(runCtx, runID, run, req.Problem, personas, runCfg)

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
		newTurns, adjustedCursor, done, stopped, resp, runErr := run.snapshot(cursor)
		cursor = adjustedCursor
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

func (a *App) executeDebateRun(ctx context.Context, runID string, run *debateRun, problem string, personas []persona.Persona, runCfg *orchestrator.Config) {
	resp, err := a.runAndSaveDebate(ctx, problem, personas, runCfg, run.appendTurn)
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
