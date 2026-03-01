package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"debate/internal/orchestrator"
	"debate/internal/output"
	"debate/internal/persona"
)

const (
	defaultAddr       = ":8080"
	maxRequestBytes   = 2 * 1024 * 1024
	serverStopTimeout = 5 * time.Second
)

type Runner interface {
	Run(ctx context.Context, problem string, personas []persona.Persona, onTurn func(orchestrator.Turn)) (orchestrator.Result, error)
}

type LoaderFunc func(path string) ([]persona.Persona, error)

type Config struct {
	PersonaPath string
	OutputDir   string
	Runner      Runner
	Loader      LoaderFunc
	Now         func() time.Time
}

type App struct {
	personaPath string
	outputDir   string
	runner      Runner
	loader      LoaderFunc
	now         func() time.Time
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

func NewApp(cfg Config) *App {
	if cfg.Loader == nil {
		cfg.Loader = persona.LoadFromFile
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &App{
		personaPath: cfg.PersonaPath,
		outputDir:   cfg.OutputDir,
		runner:      cfg.Runner,
		loader:      cfg.Loader,
		now:         cfg.Now,
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
	mux.HandleFunc("/", a.handleIndex)
	mux.HandleFunc("/api/personas", a.handlePersonas)
	mux.HandleFunc("/api/debate", a.handleDebate)
	mux.HandleFunc("/api/debate/stream", a.handleDebateStream)
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

	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		path = a.personaPath
	}
	personas, err := a.loader(path)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("load personas: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, personasResponse{
		Path:     path,
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

	var req debateRequest
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	problem := strings.TrimSpace(req.Problem)
	if problem == "" {
		writeError(w, http.StatusBadRequest, "problem is required")
		return
	}

	personas, _, err := a.resolvePersonas(req.PersonaPath, req.Personas)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("load personas: %v", err))
		return
	}

	result, err := a.runner.Run(r.Context(), problem, personas, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("run debate: %v", err))
		return
	}

	savePath := output.NewTimestampPath(a.outputDir, a.now())
	if err := output.SaveResult(savePath, result); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("save result: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, debateResponse{
		Result:            result,
		SavedJSONPath:     savePath,
		SavedMarkdownPath: output.MarkdownPath(savePath),
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

	problem := strings.TrimSpace(r.URL.Query().Get("problem"))
	if problem == "" {
		writeError(w, http.StatusBadRequest, "problem is required")
		return
	}

	personaPath := strings.TrimSpace(r.URL.Query().Get("persona_path"))
	personas, resolvedPath, err := a.resolvePersonas(personaPath, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("load personas: %v", err))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	if err := writeSSE(w, flusher, "start", streamStartEvent{
		Problem:      problem,
		PersonaPath:  resolvedPath,
		PersonaCount: len(personas),
	}); err != nil {
		return
	}

	streamCtx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var streamWriteErr error
	result, err := a.runner.Run(streamCtx, problem, personas, func(turn orchestrator.Turn) {
		if streamWriteErr != nil {
			return
		}
		if writeErr := writeSSE(w, flusher, "turn", turn); writeErr != nil {
			streamWriteErr = writeErr
			cancel()
		}
	})
	if streamWriteErr != nil {
		return
	}
	if err != nil {
		_ = writeSSE(w, flusher, "debate_error", map[string]string{
			"error": fmt.Sprintf("run debate: %v", err),
		})
		return
	}

	savePath := output.NewTimestampPath(a.outputDir, a.now())
	if err := output.SaveResult(savePath, result); err != nil {
		_ = writeSSE(w, flusher, "debate_error", map[string]string{
			"error": fmt.Sprintf("save result: %v", err),
		})
		return
	}

	_ = writeSSE(w, flusher, "complete", debateResponse{
		Result:            result,
		SavedJSONPath:     savePath,
		SavedMarkdownPath: output.MarkdownPath(savePath),
	})
}

func (a *App) resolvePersonas(personaPath string, inline []persona.Persona) ([]persona.Persona, string, error) {
	if len(inline) > 0 {
		return inline, "", nil
	}

	path := strings.TrimSpace(personaPath)
	if path == "" {
		path = a.personaPath
	}
	personas, err := a.loader(path)
	if err != nil {
		return nil, path, err
	}
	return personas, path, nil
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

const indexHTML = `<!doctype html>
<html lang="ko">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Debate Web</title>
  <style>
    :root {
      --bg: #f6f7f9;
      --panel: #ffffff;
      --ink: #1f2937;
      --muted: #6b7280;
      --accent: #0f766e;
      --accent-dark: #0a4e4a;
      --line: #d1d5db;
      --warn: #b91c1c;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "IBM Plex Sans KR", "Apple SD Gothic Neo", "Malgun Gothic", sans-serif;
      color: var(--ink);
      background:
        radial-gradient(circle at 0% 0%, #d7f5ec 0%, transparent 40%),
        radial-gradient(circle at 100% 0%, #e8f0ff 0%, transparent 36%),
        var(--bg);
      min-height: 100vh;
    }
    .wrap {
      max-width: 960px;
      margin: 0 auto;
      padding: 24px 16px 48px;
    }
    h1 {
      margin: 8px 0 4px;
      font-size: 28px;
      letter-spacing: -0.03em;
    }
    .desc {
      margin: 0 0 20px;
      color: var(--muted);
    }
    .panel {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 12px;
      padding: 16px;
      margin-bottom: 14px;
      box-shadow: 0 6px 20px rgba(0,0,0,0.05);
    }
    label {
      display: block;
      font-weight: 600;
      margin-bottom: 8px;
    }
    textarea, input {
      width: 100%;
      border: 1px solid var(--line);
      border-radius: 10px;
      padding: 10px 12px;
      font-size: 15px;
      line-height: 1.4;
      background: #fff;
    }
    textarea { min-height: 108px; resize: vertical; }
    .row {
      display: grid;
      grid-template-columns: 1fr 160px;
      gap: 12px;
      align-items: end;
    }
    button {
      border: 0;
      border-radius: 10px;
      background: linear-gradient(180deg, var(--accent), var(--accent-dark));
      color: #fff;
      font-weight: 700;
      padding: 11px 12px;
      cursor: pointer;
      transition: transform 120ms ease, opacity 120ms ease;
    }
    button:disabled { opacity: 0.6; cursor: wait; }
    button:hover:enabled { transform: translateY(-1px); }
    .meta {
      color: var(--muted);
      font-size: 14px;
      margin-top: 6px;
    }
    .error {
      color: var(--warn);
      margin-top: 10px;
      white-space: pre-wrap;
    }
    pre {
      margin: 0;
      white-space: pre-wrap;
      word-break: break-word;
      font-size: 14px;
      line-height: 1.5;
      background: #0b1324;
      color: #e5edf6;
      border-radius: 10px;
      padding: 14px;
    }
    @media (max-width: 740px) {
      .row { grid-template-columns: 1fr; }
      h1 { font-size: 24px; }
    }
  </style>
</head>
<body>
  <main class="wrap">
    <h1>Debate Web</h1>
    <p class="desc">CLI 엔진을 그대로 재사용해 브라우저에서 토론을 실행합니다.</p>

    <section class="panel">
      <label for="personaPath">Persona 파일 경로</label>
      <input id="personaPath" placeholder="./personas.json" />
      <div class="meta" id="personaInfo">persona 정보를 불러오는 중...</div>
    </section>

    <section class="panel">
      <label for="problem">토론 주제</label>
      <textarea id="problem" placeholder="예: 우리 팀의 온보딩 시간을 줄이기 위한 실험안을 설계해줘"></textarea>
      <div class="row">
        <div class="meta" id="statusText">대기 중</div>
        <button id="runBtn">토론 실행</button>
      </div>
      <div class="error" id="errorText"></div>
    </section>

    <section class="panel">
      <label>결과</label>
      <pre id="resultText">아직 실행 결과가 없습니다.</pre>
    </section>
  </main>
  <script>
    const personaPathEl = document.getElementById("personaPath");
    const personaInfoEl = document.getElementById("personaInfo");
    const problemEl = document.getElementById("problem");
    const runBtn = document.getElementById("runBtn");
    const statusText = document.getElementById("statusText");
    const errorText = document.getElementById("errorText");
    const resultText = document.getElementById("resultText");
    let currentStream = null;

    function closeCurrentStream() {
      if (!currentStream) return;
      currentStream.close();
      currentStream = null;
    }

    function parseJSON(text) {
      try {
        return JSON.parse(text);
      } catch (_) {
        return null;
      }
    }

    async function fetchPersonas() {
      const path = personaPathEl.value.trim();
      const url = path ? "/api/personas?path=" + encodeURIComponent(path) : "/api/personas";
      const res = await fetch(url);
      const payload = await res.json();
      if (!res.ok) throw new Error(payload.error || "persona 로딩 실패");

      const names = payload.personas.map(p => p.name + " (" + p.id + ")").join(", ");
      personaPathEl.value = payload.path || path;
      personaInfoEl.textContent = String(payload.personas.length) + "명 로드됨: " + names;
    }

    async function runDebate() {
      errorText.textContent = "";
      statusText.textContent = "토론 실행 중...";
      runBtn.disabled = true;
      closeCurrentStream();

      try {
        if (typeof EventSource === "undefined") {
          throw new Error("이 브라우저는 SSE(EventSource)를 지원하지 않습니다.");
        }

        const problem = problemEl.value.trim();
        if (!problem) throw new Error("토론 주제를 입력해 주세요.");

        const params = new URLSearchParams();
        params.set("problem", problem);
        const personaPath = personaPathEl.value.trim();
        if (personaPath) {
          params.set("persona_path", personaPath);
        }

        const lines = [];
        const renderLines = function () {
          if (lines.length === 0) {
            resultText.textContent = "아직 실행 결과가 없습니다.";
            return;
          }
          resultText.textContent = lines.join("\n");
        };
        const appendLine = function (line) {
          lines.push(line);
          renderLines();
        };

        const stream = new EventSource("/api/debate/stream?" + params.toString());
        currentStream = stream;
        let finished = false;

        stream.addEventListener("start", function (ev) {
          const payload = parseJSON(ev.data) || {};
          lines.length = 0;
          appendLine("stream_started: yes");
          appendLine("problem: " + (payload.problem || problem));
          if (payload.persona_path) {
            appendLine("persona_path: " + payload.persona_path);
          }
          appendLine("persona_count: " + String(payload.persona_count || 0));
          appendLine("");
          appendLine("turns:");
        });

        stream.addEventListener("turn", function (ev) {
          const turn = parseJSON(ev.data);
          if (!turn) {
            return;
          }
          appendLine("- [" + turn.index + "] " + turn.speaker_name + ": " + turn.content);
        });

        stream.addEventListener("complete", function (ev) {
          if (finished) {
            return;
          }
          finished = true;
          const payload = parseJSON(ev.data) || {};
          const result = payload.result || {};
          const consensus = result.consensus || {};
          appendLine("");
          appendLine("status: " + (result.status || "-"));
          appendLine("consensus_score: " + Number(consensus.score || 0).toFixed(2));
          appendLine("summary: " + (consensus.summary || "-"));
          appendLine("");
          appendLine("saved_json: " + (payload.saved_json_path || "-"));
          appendLine("saved_markdown: " + (payload.saved_markdown_path || "-"));
          statusText.textContent = "완료";
          runBtn.disabled = false;
          closeCurrentStream();
        });

        stream.addEventListener("debate_error", function (ev) {
          if (finished) {
            return;
          }
          finished = true;
          const payload = parseJSON(ev.data) || {};
          errorText.textContent = payload.error || "토론 실행 실패";
          statusText.textContent = "실패";
          runBtn.disabled = false;
          closeCurrentStream();
        });

        stream.onerror = function () {
          if (finished) {
            return;
          }
          finished = true;
          if (!errorText.textContent) {
            errorText.textContent = "스트림 연결이 종료되었습니다.";
          }
          statusText.textContent = "실패";
          runBtn.disabled = false;
          closeCurrentStream();
        };
      } catch (err) {
        errorText.textContent = String(err.message || err);
        statusText.textContent = "실패";
        runBtn.disabled = false;
      }
    }

    runBtn.addEventListener("click", runDebate);
    personaPathEl.addEventListener("change", async () => {
      try {
        await fetchPersonas();
      } catch (err) {
        personaInfoEl.textContent = "";
        errorText.textContent = String(err.message || err);
      }
    });

    fetchPersonas().catch((err) => {
      personaInfoEl.textContent = "";
      errorText.textContent = String(err.message || err);
    });
  </script>
</body>
</html>`
