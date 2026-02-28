package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"debate/internal/orchestrator"
	"debate/internal/persona"
	tea "github.com/charmbracelet/bubbletea"
)

type fakeRunner struct {
	result orchestrator.Result
	err    error
}

func (f *fakeRunner) Run(_ context.Context, _ string, personas []persona.Persona, onTurn func(orchestrator.Turn)) (orchestrator.Result, error) {
	onTurn(orchestrator.Turn{Index: 1, SpeakerID: personas[0].ID, SpeakerName: personas[0].Name, Type: orchestrator.TurnTypePersona, Content: "first"})
	onTurn(orchestrator.Turn{Index: 1, SpeakerID: orchestrator.ModeratorSpeakerID, SpeakerName: orchestrator.ModeratorSpeakerName, Type: orchestrator.TurnTypeModerator, Content: "next speaker focus"})
	if f.err != nil {
		return orchestrator.Result{}, f.err
	}
	return f.result, nil
}

func TestParseCommand(t *testing.T) {
	cmd, arg := parseCommand("/ask   reduce churn")
	if cmd != "/ask" || arg != "reduce churn" {
		t.Fatalf("unexpected parse: %q %q", cmd, arg)
	}

	cmd, arg = parseCommand("/show")
	if cmd != "/show" || arg != "" {
		t.Fatalf("unexpected parse: %q %q", cmd, arg)
	}

	cmd, arg = parseCommand("ask   growth loop")
	if cmd != "/ask" || arg != "growth loop" {
		t.Fatalf("unexpected alias parse: %q %q", cmd, arg)
	}

	cmd, arg = parseCommand("stop")
	if cmd != "/stop" || arg != "" {
		t.Fatalf("unexpected stop parse: %q %q", cmd, arg)
	}

	cmd, arg = parseCommand("/follow off")
	if cmd != "/follow" || arg != "off" {
		t.Fatalf("unexpected follow parse: %q %q", cmd, arg)
	}
}

func TestWrapLogLinesToWidth(t *testing.T) {
	content := wrapLogLinesToWidth([]string{"이것은 매우 긴 사회자 메시지입니다. 문장이 잘리지 않고 줄바꿈되어야 합니다."}, 16)
	if !strings.Contains(content, "\n") {
		t.Fatalf("expected wrapped multiline content, got %q", content)
	}
}

func TestHandleAskWithoutPersonas(t *testing.T) {
	m := newModel(context.Background(), modelConfig{
		PersonaPath: "./personas.json",
		OutputDir:   "./outputs",
		Runner:      &fakeRunner{},
		Loader: func(string) ([]persona.Persona, error) {
			return nil, errors.New("no file")
		},
		Now: time.Now,
	})

	cmd := m.handleCommand("/ask hello")
	if cmd != nil {
		t.Fatal("expected no command when personas are not loaded")
	}
	if got := m.logs[len(m.logs)-1]; got != "no personas loaded; use /load" {
		t.Fatalf("unexpected log: %s", got)
	}
}

func TestHandlePlainTextStartsDebate(t *testing.T) {
	runner := &fakeRunner{result: orchestrator.Result{Status: orchestrator.StatusMaxTurnsReached, Consensus: orchestrator.Consensus{Summary: "ok"}}}
	m := newModel(context.Background(), modelConfig{
		PersonaPath: "./personas.json",
		OutputDir:   t.TempDir(),
		MaxTurns:    8,
		Runner:      runner,
		Loader:      persona.LoadFromFile,
		Now:         time.Now,
	})
	m.personas = []persona.Persona{
		{ID: "p1", Name: "P1", Role: "r1"},
		{ID: "p2", Name: "P2", Role: "r2"},
	}

	cmd := m.handleCommand("이 기능의 활성화율을 높이려면?")
	if cmd == nil {
		t.Fatal("expected debate command for plain text input")
	}
	if !m.running {
		t.Fatal("expected running state to be true")
	}
	if m.debateCancel == nil {
		t.Fatal("expected cancel func to be set")
	}
	if !m.autoFollow {
		t.Fatal("expected auto-follow enabled on start")
	}
}

func TestRunDebateCmdSuccess(t *testing.T) {
	runner := &fakeRunner{result: orchestrator.Result{Status: orchestrator.StatusMaxTurnsReached, Consensus: orchestrator.Consensus{Summary: "done"}}}
	personas := []persona.Persona{{ID: "p1", Name: "P1", Role: "r1"}, {ID: "p2", Name: "P2", Role: "r2"}}
	now := func() time.Time { return time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC) }

	cmd := runDebateCmd(context.Background(), runner, "problem", personas, t.TempDir(), now)
	msg := cmd()
	started, ok := msg.(debateStreamStartedMsg)
	if !ok {
		t.Fatalf("unexpected msg type: %T", msg)
	}

	turnCount := 0
	var out *debateCompletedMsg
	for {
		streamMsg := listenDebateEventsCmd(started.events)()
		stream, ok := streamMsg.(debateStreamMsg)
		if !ok {
			t.Fatalf("unexpected stream msg type: %T", streamMsg)
		}
		if stream.closed {
			break
		}

		switch payload := stream.payload.(type) {
		case debateTurnMsg:
			turnCount++
		case debateCompletedMsg:
			cp := payload
			out = &cp
		default:
			t.Fatalf("unexpected payload type: %T", payload)
		}
	}

	if out == nil {
		t.Fatal("expected completion payload")
	}
	if out.err != nil {
		t.Fatalf("unexpected error: %v", out.err)
	}
	if out.result == nil || out.result.Consensus.Summary != "done" {
		t.Fatalf("unexpected result: %#v", out.result)
	}
	if turnCount != 2 {
		t.Fatalf("expected 2 streamed turns, got %d", turnCount)
	}
}

func TestStopWhenNotRunning(t *testing.T) {
	m := newModel(context.Background(), modelConfig{
		PersonaPath: "./personas.json",
		OutputDir:   "./outputs",
		MaxTurns:    8,
		Runner:      &fakeRunner{},
		Loader:      persona.LoadFromFile,
		Now:         time.Now,
	})

	cmd := m.handleCommand("/stop")
	if cmd != nil {
		t.Fatal("expected nil cmd on stop without running debate")
	}
	if !strings.Contains(m.logs[len(m.logs)-1], "no running debate") {
		t.Fatalf("unexpected log: %s", m.logs[len(m.logs)-1])
	}
}

func TestStopCancelsRunningDebate(t *testing.T) {
	m := newModel(context.Background(), modelConfig{
		PersonaPath: "./personas.json",
		OutputDir:   "./outputs",
		MaxTurns:    8,
		Runner:      &fakeRunner{},
		Loader:      persona.LoadFromFile,
		Now:         time.Now,
	})

	called := false
	m.running = true
	m.debateCancel = func() { called = true }

	cmd := m.handleCommand("/stop")
	if cmd != nil {
		t.Fatal("expected nil cmd on stop")
	}
	if !called {
		t.Fatal("expected cancel func to be called")
	}
}

func TestExitCancelsRunningDebate(t *testing.T) {
	m := newModel(context.Background(), modelConfig{
		PersonaPath: "./personas.json",
		OutputDir:   "./outputs",
		MaxTurns:    8,
		Runner:      &fakeRunner{},
		Loader:      persona.LoadFromFile,
		Now:         time.Now,
	})

	called := false
	m.running = true
	m.debateCancel = func() { called = true }

	cmd := m.handleCommand("/exit")
	if cmd == nil {
		t.Fatal("expected quit cmd on exit")
	}
	if !called {
		t.Fatal("expected cancel func to be called on exit")
	}
	if m.debateCancel != nil {
		t.Fatal("expected debateCancel to be cleared on exit")
	}
}

func TestCtrlCCancelsRunningDebate(t *testing.T) {
	m := newModel(context.Background(), modelConfig{
		PersonaPath: "./personas.json",
		OutputDir:   "./outputs",
		MaxTurns:    8,
		Runner:      &fakeRunner{},
		Loader:      persona.LoadFromFile,
		Now:         time.Now,
	})

	called := false
	m.running = true
	m.debateCancel = func() { called = true }

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected quit cmd on ctrl+c")
	}
	next, ok := updated.(model)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if !called {
		t.Fatal("expected cancel func to be called on ctrl+c")
	}
	if next.debateCancel != nil {
		t.Fatal("expected debateCancel to be cleared after ctrl+c")
	}
}

func TestFollowCommand(t *testing.T) {
	m := newModel(context.Background(), modelConfig{
		PersonaPath: "./personas.json",
		OutputDir:   "./outputs",
		MaxTurns:    8,
		Runner:      &fakeRunner{},
		Loader:      persona.LoadFromFile,
		Now:         time.Now,
	})

	m.autoFollow = true
	_ = m.handleCommand("/follow off")
	if m.autoFollow {
		t.Fatal("expected auto-follow off")
	}
	_ = m.handleCommand("/follow on")
	if !m.autoFollow {
		t.Fatal("expected auto-follow on")
	}
}

func TestMouseWheelScrollUpdatesAutoFollow(t *testing.T) {
	m := newModel(context.Background(), modelConfig{
		PersonaPath: "./personas.json",
		OutputDir:   "./outputs",
		MaxTurns:    0,
		Runner:      &fakeRunner{},
		Loader:      persona.LoadFromFile,
		Now:         time.Now,
	})

	for i := 0; i < 120; i++ {
		m.appendLog("scroll line")
	}
	if !m.logViewport.AtBottom() {
		t.Fatal("expected viewport at bottom after initial append")
	}
	if !m.autoFollow {
		t.Fatal("expected auto-follow initially on")
	}

	updated, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	afterUp, ok := updated.(model)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if afterUp.autoFollow {
		t.Fatal("expected auto-follow off after wheel up")
	}

	for i := 0; i < 200; i++ {
		updated, _ = afterUp.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
		afterUp = updated.(model)
		if afterUp.logViewport.AtBottom() {
			break
		}
	}
	if !afterUp.logViewport.AtBottom() {
		t.Fatal("expected viewport to reach bottom after wheel down")
	}
	if !afterUp.autoFollow {
		t.Fatal("expected auto-follow on when wheel down reaches bottom")
	}
}

func TestDebateStreamClosedWhileRunningEndsSession(t *testing.T) {
	m := newModel(context.Background(), modelConfig{
		PersonaPath: "./personas.json",
		OutputDir:   "./outputs",
		MaxTurns:    0,
		Runner:      &fakeRunner{},
		Loader:      persona.LoadFromFile,
		Now:         time.Now,
	})
	m.running = true
	m.debateCancel = func() {}

	updated, _ := m.Update(debateStreamMsg{closed: true})
	next, ok := updated.(model)
	if !ok {
		t.Fatalf("unexpected model type: %T", updated)
	}
	if next.running {
		t.Fatal("expected running=false when stream closes")
	}
	if next.debateCancel != nil {
		t.Fatal("expected debateCancel to be cleared when stream closes")
	}
	if !strings.Contains(strings.Join(next.logs, "\n"), "debate stream closed") {
		t.Fatalf("expected stream closed log, got %#v", next.logs)
	}
	if !strings.Contains(strings.Join(next.logs, "\n"), "==== debate end ====") {
		t.Fatalf("expected debate end log, got %#v", next.logs)
	}
}

func TestFormatTurnLinesReadableSpacing(t *testing.T) {
	personaTurn := orchestrator.Turn{
		Index:       3,
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
	if !strings.Contains(personaLines[2], "turn 3") {
		t.Fatalf("unexpected header line: %q", personaLines[2])
	}
	if !containsLinePrefix(personaLines, "  first line") || !containsLinePrefix(personaLines, "  second line") {
		t.Fatalf("expected content block prefix, got %#v", personaLines)
	}
	if personaLines[len(personaLines)-1] != "" {
		t.Fatalf("expected trailing blank line, got %q", personaLines[len(personaLines)-1])
	}

	moderatorTurn := orchestrator.Turn{
		Index:       3,
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

func containsLinePrefix(lines []string, prefix string) bool {
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}
