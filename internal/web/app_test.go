package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"debate/internal/orchestrator"
	"debate/internal/persona"
)

type stubRunner struct {
	callCount   int
	problem     string
	personas    []persona.Persona
	result      orchestrator.Result
	streamTurns []orchestrator.Turn
	err         error
}

func (s *stubRunner) Run(_ context.Context, problem string, personas []persona.Persona, onTurn func(orchestrator.Turn)) (orchestrator.Result, error) {
	s.callCount++
	s.problem = problem
	s.personas = append([]persona.Persona(nil), personas...)
	if onTurn != nil {
		for _, turn := range s.streamTurns {
			onTurn(turn)
		}
	}
	if s.err != nil {
		return orchestrator.Result{}, s.err
	}
	return s.result, nil
}

type stoppableRunner struct {
	startOnce sync.Once
	doneOnce  sync.Once
	started   chan struct{}
	done      chan struct{}
}

func (s *stoppableRunner) Run(ctx context.Context, _ string, _ []persona.Persona, _ func(orchestrator.Turn)) (orchestrator.Result, error) {
	s.startOnce.Do(func() {
		if s.started != nil {
			close(s.started)
		}
	})
	<-ctx.Done()
	s.doneOnce.Do(func() {
		if s.done != nil {
			close(s.done)
		}
	})
	return orchestrator.Result{}, ctx.Err()
}

func TestDebateEndpointWithInlinePersonas(t *testing.T) {
	outDir := t.TempDir()
	now := time.Date(2026, 3, 1, 1, 2, 3, 4, time.UTC)
	inputPersonas := []persona.Persona{
		{ID: "architect", Name: "Architect", Role: "design"},
		{ID: "operator", Name: "Operator", Role: "operations"},
	}

	runner := &stubRunner{
		result: orchestrator.Result{
			Problem:  "test problem",
			Personas: inputPersonas,
			Turns: []orchestrator.Turn{
				{Index: 1, SpeakerID: "architect", SpeakerName: "Architect", Type: orchestrator.TurnTypePersona, Content: "first point"},
			},
			Consensus: orchestrator.Consensus{Reached: false, Score: 0.42, Summary: "no consensus"},
			Status:    orchestrator.StatusMaxTurnsReached,
		},
	}
	app := NewApp(Config{
		PersonaPath: "./personas.json",
		OutputDir:   outDir,
		Runner:      runner,
		Loader: func(string) ([]persona.Persona, error) {
			return nil, errors.New("loader should not be called")
		},
		Now: func() time.Time { return now },
	})

	body := map[string]any{
		"problem":  "test problem",
		"personas": inputPersonas,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/debate", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if runner.callCount != 1 {
		t.Fatalf("expected 1 runner call, got %d", runner.callCount)
	}
	if runner.problem != "test problem" {
		t.Fatalf("unexpected problem: %s", runner.problem)
	}
	if len(runner.personas) != 2 {
		t.Fatalf("unexpected personas: %#v", runner.personas)
	}

	var resp debateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.SavedJSONPath == "" || resp.SavedMarkdownPath == "" {
		t.Fatalf("expected saved paths, got %#v", resp)
	}
	if _, err := os.Stat(resp.SavedJSONPath); err != nil {
		t.Fatalf("saved json file not found: %v", err)
	}
	if _, err := os.Stat(resp.SavedMarkdownPath); err != nil {
		t.Fatalf("saved markdown file not found: %v", err)
	}
}

func TestDebateEndpointAvoidsOutputPathCollision(t *testing.T) {
	outDir := t.TempDir()
	fixedNow := time.Date(2026, 3, 1, 1, 2, 3, 4, time.UTC)
	runner := &stubRunner{
		result: orchestrator.Result{
			Problem: "collision test",
			Personas: []persona.Persona{
				{ID: "p1", Name: "Planner", Role: "plan"},
				{ID: "p2", Name: "Builder", Role: "build"},
			},
			Status:    orchestrator.StatusMaxTurnsReached,
			Consensus: orchestrator.Consensus{Summary: "done"},
		},
	}
	app := NewApp(Config{
		PersonaPath: "./personas.json",
		OutputDir:   outDir,
		Runner:      runner,
		Loader: func(string) ([]persona.Persona, error) {
			return []persona.Persona{
				{ID: "p1", Name: "Planner", Role: "plan"},
				{ID: "p2", Name: "Builder", Role: "build"},
			}, nil
		},
		Now: func() time.Time { return fixedNow },
	})

	makeRequest := func() debateResponse {
		req := httptest.NewRequest(http.MethodPost, "/api/debate", bytes.NewBufferString(`{"problem":"collision test"}`))
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
		}
		var resp debateResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return resp
	}

	first := makeRequest()
	second := makeRequest()

	if first.SavedJSONPath == second.SavedJSONPath {
		t.Fatalf("expected different json paths, got same path %s", first.SavedJSONPath)
	}
	if first.SavedMarkdownPath == second.SavedMarkdownPath {
		t.Fatalf("expected different markdown paths, got same path %s", first.SavedMarkdownPath)
	}
	if _, err := os.Stat(first.SavedJSONPath); err != nil {
		t.Fatalf("first json file missing: %v", err)
	}
	if _, err := os.Stat(second.SavedJSONPath); err != nil {
		t.Fatalf("second json file missing: %v", err)
	}
}

func TestNextOutputPathIsUniqueUnderConcurrency(t *testing.T) {
	app := NewApp(Config{
		PersonaPath: "./personas.json",
		OutputDir:   t.TempDir(),
		Runner:      &stubRunner{},
		Loader: func(string) ([]persona.Persona, error) {
			return nil, nil
		},
		Now: func() time.Time {
			return time.Date(2026, 3, 1, 1, 2, 3, 4, time.UTC)
		},
	})

	const n = 120
	type result struct {
		path string
		err  error
	}
	out := make(chan result, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			path, err := app.nextOutputPath()
			out <- result{path: path, err: err}
		}()
	}
	wg.Wait()
	close(out)

	seen := make(map[string]struct{}, n)
	for r := range out {
		if r.err != nil {
			t.Fatalf("nextOutputPath returned error: %v", r.err)
		}
		if _, exists := seen[r.path]; exists {
			t.Fatalf("duplicate path generated: %s", r.path)
		}
		seen[r.path] = struct{}{}
	}
	if len(seen) != n {
		t.Fatalf("expected %d unique paths, got %d", n, len(seen))
	}
}

func TestDebateEndpointLoadsPersonasByPath(t *testing.T) {
	runner := &stubRunner{
		result: orchestrator.Result{
			Status:    orchestrator.StatusConsensusReached,
			Consensus: orchestrator.Consensus{Reached: true, Score: 1.0, Summary: "done"},
		},
	}

	loadedPath := ""
	loadedPersonas := []persona.Persona{
		{ID: "a", Name: "A", Role: "one"},
		{ID: "b", Name: "B", Role: "two"},
	}
	app := NewApp(Config{
		PersonaPath: "./default-personas.json",
		OutputDir:   t.TempDir(),
		Runner:      runner,
		Loader: func(path string) ([]persona.Persona, error) {
			loadedPath = path
			return loadedPersonas, nil
		},
		Now: time.Now,
	})

	reqBody := `{"problem":"path based load","persona_path":"./custom.json"}`
	req := httptest.NewRequest(http.MethodPost, "/api/debate", bytes.NewBufferString(reqBody))
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if !filepath.IsAbs(loadedPath) {
		t.Fatalf("expected absolute loader path, got %s", loadedPath)
	}
	if filepath.Base(loadedPath) != "custom.json" {
		t.Fatalf("unexpected loader path: %s", loadedPath)
	}
	if len(runner.personas) != len(loadedPersonas) {
		t.Fatalf("runner personas mismatch: %#v", runner.personas)
	}
}

func TestDebateEndpointValidatesProblem(t *testing.T) {
	app := NewApp(Config{
		PersonaPath: "./personas.json",
		OutputDir:   t.TempDir(),
		Runner:      &stubRunner{},
		Loader: func(string) ([]persona.Persona, error) {
			return nil, nil
		},
		Now: time.Now,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/debate", bytes.NewBufferString(`{"problem":"   "}`))
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDebateEndpointRejectsUnknownField(t *testing.T) {
	app := NewApp(Config{
		PersonaPath: "./personas.json",
		OutputDir:   t.TempDir(),
		Runner:      &stubRunner{},
		Loader: func(string) ([]persona.Persona, error) {
			return []persona.Persona{
				{ID: "p1", Name: "Planner", Role: "plan"},
				{ID: "p2", Name: "Builder", Role: "build"},
			}, nil
		},
		Now: time.Now,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/debate", bytes.NewBufferString(`{"problem":"ok","unexpected":"x"}`))
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDebateEndpointRejectsMultipleJSONValues(t *testing.T) {
	app := NewApp(Config{
		PersonaPath: "./personas.json",
		OutputDir:   t.TempDir(),
		Runner:      &stubRunner{},
		Loader: func(string) ([]persona.Persona, error) {
			return []persona.Persona{
				{ID: "p1", Name: "Planner", Role: "plan"},
				{ID: "p2", Name: "Builder", Role: "build"},
			}, nil
		},
		Now: time.Now,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/debate", bytes.NewBufferString(`{"problem":"ok"}{"problem":"no"}`))
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDebateEndpointRejectsInvalidInlinePersonas(t *testing.T) {
	runner := &stubRunner{}
	app := NewApp(Config{
		PersonaPath: "./personas.json",
		OutputDir:   t.TempDir(),
		Runner:      runner,
		Loader: func(string) ([]persona.Persona, error) {
			return nil, errors.New("loader should not be called")
		},
		Now: time.Now,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/debate", bytes.NewBufferString(`{
		"problem":"inline validation",
		"personas":[{"id":"p1","name":"Only One","role":"solo"}]
	}`))
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if runner.callCount != 0 {
		t.Fatalf("runner must not be called, got %d", runner.callCount)
	}
}

func TestDebateEndpointRejectsPersonaPathAndInlinePersonasTogether(t *testing.T) {
	runner := &stubRunner{}
	app := NewApp(Config{
		PersonaPath: "./personas.json",
		OutputDir:   t.TempDir(),
		Runner:      runner,
		Loader: func(string) ([]persona.Persona, error) {
			return nil, errors.New("loader should not be called")
		},
		Now: time.Now,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/debate", bytes.NewBufferString(`{
		"problem":"conflict payload",
		"persona_path":"./custom.json",
		"personas":[
			{"id":"p1","name":"Planner","role":"plan"},
			{"id":"p2","name":"Builder","role":"build"}
		]
	}`))
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if runner.callCount != 0 {
		t.Fatalf("runner must not be called, got %d", runner.callCount)
	}
}

func TestDebateStreamStartAndSubscribeStreamsTurnsAndComplete(t *testing.T) {
	loadedPath := ""
	loadedPersonas := []persona.Persona{
		{ID: "p1", Name: "Planner", Role: "plan"},
		{ID: "p2", Name: "Builder", Role: "build"},
	}
	runner := &stubRunner{
		streamTurns: []orchestrator.Turn{
			{Index: 1, SpeakerID: "p1", SpeakerName: "Planner", Type: orchestrator.TurnTypePersona, Content: "first"},
			{Index: 2, SpeakerID: "p2", SpeakerName: "Builder", Type: orchestrator.TurnTypePersona, Content: "second"},
		},
		result: orchestrator.Result{
			Problem: "stream test",
			Consensus: orchestrator.Consensus{
				Score:   0.88,
				Summary: "almost there",
			},
			Status: orchestrator.StatusMaxTurnsReached,
		},
	}
	app := NewApp(Config{
		PersonaPath: "./personas.json",
		OutputDir:   t.TempDir(),
		Runner:      runner,
		Loader: func(path string) ([]persona.Persona, error) {
			loadedPath = path
			return loadedPersonas, nil
		},
		Now: time.Now,
	})

	startReq := httptest.NewRequest(http.MethodPost, "/api/debate/stream/start", bytes.NewBufferString(`{
		"problem":"stream test",
		"persona_path":"./custom.json"
	}`))
	startRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(startRec, startReq)

	if startRec.Code != http.StatusAccepted {
		t.Fatalf("unexpected start status: %d body=%s", startRec.Code, startRec.Body.String())
	}
	var started streamStartResponse
	if err := json.Unmarshal(startRec.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	if strings.TrimSpace(started.RunID) == "" {
		t.Fatalf("missing run_id: %#v", started)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/debate/stream?run_id="+started.RunID, nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if !filepath.IsAbs(loadedPath) {
		t.Fatalf("expected absolute loader path, got %s", loadedPath)
	}
	if filepath.Base(loadedPath) != "custom.json" {
		t.Fatalf("unexpected loader path: %s", loadedPath)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("unexpected content type: %s", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: start") {
		t.Fatalf("missing start event: %s", body)
	}
	if !strings.Contains(body, "event: turn") {
		t.Fatalf("missing turn event: %s", body)
	}
	if !strings.Contains(body, "event: complete") {
		t.Fatalf("missing complete event: %s", body)
	}
	if !strings.Contains(body, "\"speaker_name\":\"Planner\"") {
		t.Fatalf("missing streamed turn payload: %s", body)
	}
	if !strings.Contains(body, "\"saved_json_path\"") {
		t.Fatalf("missing completion payload: %s", body)
	}
}

func TestDebateStreamStartEndpointValidatesProblem(t *testing.T) {
	app := NewApp(Config{
		PersonaPath: "./personas.json",
		OutputDir:   t.TempDir(),
		Runner:      &stubRunner{},
		Loader: func(string) ([]persona.Persona, error) {
			return []persona.Persona{
				{ID: "p1", Name: "Planner", Role: "plan"},
				{ID: "p2", Name: "Builder", Role: "build"},
			}, nil
		},
		Now: time.Now,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/debate/stream/start", bytes.NewBufferString(`{"problem":"   "}`))
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDebateStreamSubscribeRequiresRunID(t *testing.T) {
	app := NewApp(Config{
		PersonaPath: "./personas.json",
		OutputDir:   t.TempDir(),
		Runner:      &stubRunner{},
		Loader: func(string) ([]persona.Persona, error) {
			return nil, nil
		},
		Now: time.Now,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/debate/stream", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDebateStreamSubscribeUnknownRunID(t *testing.T) {
	runner := &stubRunner{}
	app := NewApp(Config{
		PersonaPath: "./personas.json",
		OutputDir:   t.TempDir(),
		Runner:      runner,
		Loader: func(string) ([]persona.Persona, error) {
			return nil, nil
		},
		Now: time.Now,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/debate/stream?run_id=missing", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if runner.callCount != 0 {
		t.Fatalf("runner must not be called, got %d", runner.callCount)
	}
}

func TestDebateStreamStopEndpointCancelsRun(t *testing.T) {
	blocking := &stoppableRunner{
		started: make(chan struct{}),
		done:    make(chan struct{}),
	}
	app := NewApp(Config{
		PersonaPath: "./personas.json",
		OutputDir:   t.TempDir(),
		Runner:      blocking,
		Loader: func(string) ([]persona.Persona, error) {
			return []persona.Persona{
				{ID: "p1", Name: "Planner", Role: "plan"},
				{ID: "p2", Name: "Builder", Role: "build"},
			}, nil
		},
		Now: time.Now,
	})

	startReq := httptest.NewRequest(http.MethodPost, "/api/debate/stream/start", bytes.NewBufferString(`{"problem":"stop test"}`))
	startRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusAccepted {
		t.Fatalf("unexpected start status: %d body=%s", startRec.Code, startRec.Body.String())
	}

	var started streamStartResponse
	if err := json.Unmarshal(startRec.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	if strings.TrimSpace(started.RunID) == "" {
		t.Fatalf("missing run_id: %#v", started)
	}

	select {
	case <-blocking.started:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not start")
	}

	stopReq := httptest.NewRequest(http.MethodPost, "/api/debate/stream/stop", bytes.NewBufferString(`{"run_id":"`+started.RunID+`"}`))
	stopRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(stopRec, stopReq)
	if stopRec.Code != http.StatusOK {
		t.Fatalf("unexpected stop status: %d body=%s", stopRec.Code, stopRec.Body.String())
	}

	select {
	case <-blocking.done:
	case <-time.After(2 * time.Second):
		t.Fatal("runner was not canceled")
	}

	streamReq := httptest.NewRequest(http.MethodGet, "/api/debate/stream?run_id="+started.RunID, nil)
	streamRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(streamRec, streamReq)
	if streamRec.Code != http.StatusOK {
		t.Fatalf("unexpected stream status: %d body=%s", streamRec.Code, streamRec.Body.String())
	}
	body := streamRec.Body.String()
	if !strings.Contains(body, "event: stopped") {
		t.Fatalf("missing stopped event: %s", body)
	}
}

func TestDebateStreamRunTimeoutEmitsDebateError(t *testing.T) {
	blocking := &stoppableRunner{
		started: make(chan struct{}),
		done:    make(chan struct{}),
	}
	app := NewApp(Config{
		PersonaPath: "./personas.json",
		OutputDir:   t.TempDir(),
		Runner:      blocking,
		Loader: func(string) ([]persona.Persona, error) {
			return []persona.Persona{
				{ID: "p1", Name: "Planner", Role: "plan"},
				{ID: "p2", Name: "Builder", Role: "build"},
			}, nil
		},
		Now:        time.Now,
		RunTimeout: 60 * time.Millisecond,
	})

	startReq := httptest.NewRequest(http.MethodPost, "/api/debate/stream/start", bytes.NewBufferString(`{"problem":"timeout test"}`))
	startRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusAccepted {
		t.Fatalf("unexpected start status: %d body=%s", startRec.Code, startRec.Body.String())
	}

	var started streamStartResponse
	if err := json.Unmarshal(startRec.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	if strings.TrimSpace(started.RunID) == "" {
		t.Fatalf("missing run_id: %#v", started)
	}

	select {
	case <-blocking.started:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not start")
	}

	streamReq := httptest.NewRequest(http.MethodGet, "/api/debate/stream?run_id="+started.RunID, nil)
	streamRec := httptest.NewRecorder()
	app.Handler().ServeHTTP(streamRec, streamReq)
	if streamRec.Code != http.StatusOK {
		t.Fatalf("unexpected stream status: %d body=%s", streamRec.Code, streamRec.Body.String())
	}
	body := streamRec.Body.String()
	if !strings.Contains(body, "event: debate_error") {
		t.Fatalf("expected debate_error event on timeout, body=%s", body)
	}
	if !strings.Contains(body, "context deadline exceeded") {
		t.Fatalf("expected context deadline error detail, body=%s", body)
	}

	select {
	case <-blocking.done:
	case <-time.After(2 * time.Second):
		t.Fatal("runner was not canceled by timeout")
	}
}

func TestPersonasEndpointMethodNotAllowed(t *testing.T) {
	app := NewApp(Config{
		PersonaPath: "./personas.json",
		OutputDir:   t.TempDir(),
		Runner:      &stubRunner{},
		Loader: func(string) ([]persona.Persona, error) {
			return nil, nil
		},
		Now: time.Now,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/personas", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("unexpected allow header: %s", allow)
	}
}

func TestIndexEndpointServed(t *testing.T) {
	app := NewApp(Config{
		PersonaPath: "./personas.json",
		OutputDir:   filepath.Join(t.TempDir(), "out"),
		Runner:      &stubRunner{},
		Loader: func(string) ([]persona.Persona, error) {
			return nil, nil
		},
		Now: time.Now,
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct == "" {
		t.Fatal("missing content type")
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("Debate Web")) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestStaticAssetServed(t *testing.T) {
	app := NewApp(Config{
		PersonaPath: "./personas.json",
		OutputDir:   t.TempDir(),
		Runner:      &stubRunner{},
		Loader: func(string) ([]persona.Persona, error) {
			return nil, nil
		},
		Now: time.Now,
	})

	req := httptest.NewRequest(http.MethodGet, "/static/app.js", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "const predefinedGroups") {
		t.Fatalf("unexpected static content: %s", rec.Body.String())
	}
}

func TestPersonasEndpointRejectsPathTraversal(t *testing.T) {
	loaderCalled := false
	app := NewApp(Config{
		PersonaPath: "./personas.json",
		BaseDir:     filepath.Join(t.TempDir(), "project"),
		OutputDir:   t.TempDir(),
		Runner:      &stubRunner{},
		Loader: func(string) ([]persona.Persona, error) {
			loaderCalled = true
			return nil, nil
		},
		Now: time.Now,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/personas?path=../secrets.json", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if loaderCalled {
		t.Fatal("loader must not be called for invalid path")
	}
}

func TestPersonasEndpointRejectsSymlinkEscape(t *testing.T) {
	projectDir := t.TempDir()
	outsideDir := t.TempDir()
	outsidePersonas := filepath.Join(outsideDir, "outside.json")
	if err := os.WriteFile(outsidePersonas, []byte("[]"), 0o644); err != nil {
		t.Fatalf("write outside personas: %v", err)
	}

	linkPath := filepath.Join(projectDir, "link.json")
	if err := os.Symlink(outsidePersonas, linkPath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	loaderCalled := false
	app := NewApp(Config{
		PersonaPath: "./personas.json",
		BaseDir:     projectDir,
		OutputDir:   t.TempDir(),
		Runner:      &stubRunner{},
		Loader: func(string) ([]persona.Persona, error) {
			loaderCalled = true
			return nil, nil
		},
		Now: time.Now,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/personas?path=./link.json", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if loaderCalled {
		t.Fatal("loader must not be called for symlink escape path")
	}
}

func TestPersonasEndpointRejectsNonJSONPath(t *testing.T) {
	loaderCalled := false
	app := NewApp(Config{
		PersonaPath: "./personas.json",
		OutputDir:   t.TempDir(),
		Runner:      &stubRunner{},
		Loader: func(string) ([]persona.Persona, error) {
			loaderCalled = true
			return nil, nil
		},
		Now: time.Now,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/personas?path=./personas.txt", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if loaderCalled {
		t.Fatal("loader must not be called for invalid extension")
	}
}
