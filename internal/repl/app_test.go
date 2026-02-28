package repl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"debate/internal/orchestrator"
	"debate/internal/persona"
)

type fakeRunner struct {
	calls []string
}

func (f *fakeRunner) Run(_ context.Context, problem string, personas []persona.Persona, onTurn func(orchestrator.Turn)) (orchestrator.Result, error) {
	f.calls = append(f.calls, problem)
	onTurn(orchestrator.Turn{Index: 1, SpeakerID: personas[0].ID, SpeakerName: personas[0].Name, Content: "first point"})
	return orchestrator.Result{
		Problem:  problem,
		Personas: personas,
		Turns: []orchestrator.Turn{
			{Index: 1, SpeakerID: personas[0].ID, SpeakerName: personas[0].Name, Content: "first point"},
		},
		Consensus: orchestrator.Consensus{Reached: false, Score: 0.4, Summary: "No consensus yet"},
		Status:    orchestrator.StatusMaxTurnsReached,
	}, nil
}

func TestParseCommandSupportsTabs(t *testing.T) {
	cmd, arg := parseCommand("ask\tgrowth experiment")
	if cmd != "/ask" || arg != "growth experiment" {
		t.Fatalf("unexpected ask parse: %q %q", cmd, arg)
	}

	cmd, arg = parseCommand("/show\t")
	if cmd != "/show" || arg != "" {
		t.Fatalf("unexpected show parse: %q %q", cmd, arg)
	}
}

func TestStartAskTwiceCreatesTwoResults(t *testing.T) {
	tmp := t.TempDir()
	personaPath := filepath.Join(tmp, "personas.json")
	content := `[
  {"id":"architect","name":"Architect","role":"design"},
  {"id":"operator","name":"Operator","role":"operations"}
]`
	if err := os.WriteFile(personaPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write personas: %v", err)
	}

	runner := &fakeRunner{}
	var out strings.Builder
	counter := 0
	base := time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC)

	app := NewApp(Config{
		PersonaPath: personaPath,
		OutputDir:   filepath.Join(tmp, "outputs"),
		Runner:      runner,
		Loader:      persona.LoadFromFile,
		Writer:      &out,
		Now: func() time.Time {
			counter++
			return base.Add(time.Duration(counter) * time.Second)
		},
	})

	input := "/show\n/ask first question\n/ask second question\n/exit\n"
	if err := app.Start(context.Background(), strings.NewReader(input)); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if len(runner.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(runner.calls))
	}
	if runner.calls[0] != "first question" || runner.calls[1] != "second question" {
		t.Fatalf("unexpected calls: %#v", runner.calls)
	}

	files, err := filepath.Glob(filepath.Join(tmp, "outputs", "*-debate.json"))
	if err != nil {
		t.Fatalf("glob outputs: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 output files, got %d", len(files))
	}
}

func TestAskWithoutPersonas(t *testing.T) {
	runner := &fakeRunner{}
	var out strings.Builder

	app := NewApp(Config{
		PersonaPath: "missing.json",
		OutputDir:   t.TempDir(),
		Runner:      runner,
		Loader: func(string) ([]persona.Persona, error) {
			return nil, os.ErrNotExist
		},
		Writer: &out,
		Now:    time.Now,
	})

	if err := app.Start(context.Background(), strings.NewReader("/ask test\n/exit\n")); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if len(runner.calls) != 0 {
		t.Fatalf("expected no runner calls, got %d", len(runner.calls))
	}
	if !strings.Contains(out.String(), "no personas loaded") {
		t.Fatalf("expected missing personas message, output=%q", out.String())
	}
}

func TestUnknownAskPrefixCommand(t *testing.T) {
	tmp := t.TempDir()
	personaPath := filepath.Join(tmp, "personas.json")
	content := `[
  {"id":"architect","name":"Architect","role":"design"},
  {"id":"operator","name":"Operator","role":"operations"}
]`
	if err := os.WriteFile(personaPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write personas: %v", err)
	}

	runner := &fakeRunner{}
	var out strings.Builder
	app := NewApp(Config{
		PersonaPath: personaPath,
		OutputDir:   filepath.Join(tmp, "outputs"),
		Runner:      runner,
		Loader:      persona.LoadFromFile,
		Writer:      &out,
		Now:         time.Now,
	})

	if err := app.Start(context.Background(), strings.NewReader("/askanything\n/exit\n")); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if len(runner.calls) != 0 {
		t.Fatalf("expected no runner calls, got %d", len(runner.calls))
	}
	if !strings.Contains(out.String(), "unknown command") {
		t.Fatalf("expected unknown command message, output=%q", out.String())
	}
}

func TestStartWithNilRunner(t *testing.T) {
	app := NewApp(Config{
		PersonaPath: "personas.json",
		OutputDir:   t.TempDir(),
		Loader:      persona.LoadFromFile,
		Writer:      &strings.Builder{},
		Now:         time.Now,
	})

	err := app.Start(context.Background(), strings.NewReader("/exit\n"))
	if err == nil {
		t.Fatal("expected runner error")
	}
	if !strings.Contains(err.Error(), "runner is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPlainTextStartsDebate(t *testing.T) {
	tmp := t.TempDir()
	personaPath := filepath.Join(tmp, "personas.json")
	content := `[
  {"id":"architect","name":"Architect","role":"design"},
  {"id":"operator","name":"Operator","role":"operations"}
]`
	if err := os.WriteFile(personaPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write personas: %v", err)
	}

	runner := &fakeRunner{}
	var out strings.Builder
	app := NewApp(Config{
		PersonaPath: personaPath,
		OutputDir:   filepath.Join(tmp, "outputs"),
		Runner:      runner,
		Loader:      persona.LoadFromFile,
		Writer:      &out,
		Now:         time.Now,
	})

	if err := app.Start(context.Background(), strings.NewReader("활성화율 개선 아이디어\n/exit\n")); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if len(runner.calls) != 1 || runner.calls[0] != "활성화율 개선 아이디어" {
		t.Fatalf("unexpected runner calls: %#v", runner.calls)
	}
}

func TestAskAliasWithoutSlash(t *testing.T) {
	tmp := t.TempDir()
	personaPath := filepath.Join(tmp, "personas.json")
	content := `[
  {"id":"architect","name":"Architect","role":"design"},
  {"id":"operator","name":"Operator","role":"operations"}
]`
	if err := os.WriteFile(personaPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write personas: %v", err)
	}

	runner := &fakeRunner{}
	var out strings.Builder
	app := NewApp(Config{
		PersonaPath: personaPath,
		OutputDir:   filepath.Join(tmp, "outputs"),
		Runner:      runner,
		Loader:      persona.LoadFromFile,
		Writer:      &out,
		Now:         time.Now,
	})

	if err := app.Start(context.Background(), strings.NewReader("ask 성장실험 설계\n/exit\n")); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if len(runner.calls) != 1 || runner.calls[0] != "성장실험 설계" {
		t.Fatalf("unexpected runner calls: %#v", runner.calls)
	}
}

func TestFormatTurnLinesReadableSpacing(t *testing.T) {
	personaTurn := orchestrator.Turn{
		Index:       2,
		SpeakerID:   "p1",
		SpeakerName: "Persona A",
		Type:        orchestrator.TurnTypePersona,
		Content:     "first line\n\nsecond line",
	}
	personaLines := formatTurnLines(personaTurn)
	if len(personaLines) < 7 {
		t.Fatalf("expected richer turn block, got %#v", personaLines)
	}
	if personaLines[0] != "" {
		t.Fatalf("expected leading blank line, got %q", personaLines[0])
	}
	if !strings.Contains(personaLines[1], "---") {
		t.Fatalf("expected persona separator, got %q", personaLines[1])
	}
	if !strings.Contains(personaLines[2], "turn 2 | Persona A") {
		t.Fatalf("unexpected header line: %q", personaLines[2])
	}
	if !containsPrefix(personaLines, "  first line") || !containsPrefix(personaLines, "  second line") {
		t.Fatalf("expected content block prefix, got %#v", personaLines)
	}
	if personaLines[len(personaLines)-1] != "" {
		t.Fatalf("expected trailing blank line, got %q", personaLines[len(personaLines)-1])
	}

	moderatorTurn := orchestrator.Turn{
		Index:       2,
		SpeakerID:   orchestrator.ModeratorSpeakerID,
		SpeakerName: orchestrator.ModeratorSpeakerName,
		Type:        orchestrator.TurnTypeModerator,
		Content:     "moderator note",
	}
	moderatorLines := formatTurnLines(moderatorTurn)
	if !strings.Contains(moderatorLines[1], "===") {
		t.Fatalf("expected moderator separator, got %q", moderatorLines[1])
	}
}

func containsPrefix(lines []string, prefix string) bool {
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}
