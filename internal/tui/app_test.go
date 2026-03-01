package tui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"debate/internal/orchestrator"
	"debate/internal/persona"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

	cmd, arg = parseCommand("/ask\tnew market strategy")
	if cmd != "/ask" || arg != "new market strategy" {
		t.Fatalf("unexpected tab parse: %q %q", cmd, arg)
	}
}

func TestWrapLogLinesToWidth(t *testing.T) {
	content := wrapLogLinesToWidth([]string{"이것은 매우 긴 사회자 메시지입니다. 문장이 잘리지 않고 줄바꿈되어야 합니다."}, 16)
	if !strings.Contains(content, "\n") {
		t.Fatalf("expected wrapped multiline content, got %q", content)
	}
}

func TestNewModelDefaultsLoaderAndClock(t *testing.T) {
	m := newModel(context.Background(), modelConfig{
		PersonaPath: "./personas.json",
		OutputDir:   "./outputs",
		Runner:      &fakeRunner{},
	})
	if m.loader == nil {
		t.Fatal("expected default loader to be set")
	}
	if m.now == nil {
		t.Fatal("expected default clock to be set")
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

func TestBuildPersonaPanelShowsTurnCountsAndLastSpeaker(t *testing.T) {
	m := newModel(context.Background(), modelConfig{
		PersonaPath: "./personas.json",
		OutputDir:   "./outputs",
		MaxTurns:    8,
		Runner:      &fakeRunner{},
		Loader:      persona.LoadFromFile,
		Now:         time.Now,
	})
	m.personas = []persona.Persona{
		{ID: "p1", Name: "Alpha", Role: "growth", Stance: "aggressive"},
		{ID: "p2", Name: "Beta", Role: "ops", Stance: "stable"},
	}
	m.speakerTurns["p1"] = 3
	m.speakerTurns["p2"] = 1
	m.lastSpeakerName = "Alpha"

	panel := m.buildPersonaPanel(50, 24)
	if !strings.Contains(panel, "[3T]") || !strings.Contains(panel, "[1T]") {
		t.Fatalf("expected turn counters in panel, got %q", panel)
	}
	if !strings.Contains(panel, ">  1)") {
		t.Fatalf("expected last-speaker marker, got %q", panel)
	}
	if !strings.Contains(panel, "last speaker: Alpha") {
		t.Fatalf("expected last speaker summary, got %q", panel)
	}
}

func TestBuildPersonaPanelCompactsWhenTooManyPersonas(t *testing.T) {
	m := newModel(context.Background(), modelConfig{
		PersonaPath: "./personas.json",
		OutputDir:   "./outputs",
		MaxTurns:    8,
		Runner:      &fakeRunner{},
		Loader:      persona.LoadFromFile,
		Now:         time.Now,
	})
	m.personas = []persona.Persona{
		{ID: "p1", Name: "Alpha", Role: "r1"},
		{ID: "p2", Name: "Beta", Role: "r2"},
		{ID: "p3", Name: "Gamma", Role: "r3"},
		{ID: "p4", Name: "Delta", Role: "r4"},
		{ID: "p5", Name: "Epsilon", Role: "r5"},
		{ID: "p6", Name: "Zeta", Role: "r6"},
	}

	panel := m.buildPersonaPanel(32, 4)
	if !strings.Contains(panel, "+") || !strings.Contains(panel, "more personas") {
		t.Fatalf("expected overflow summary for compact panel, got %q", panel)
	}
	if len(strings.Split(panel, "\n")) > 4 {
		t.Fatalf("expected panel lines to respect maxLines, got %q", panel)
	}
}

func TestBuildPersonaPanelNarrowWidthDoesNotWrap(t *testing.T) {
	m := newModel(context.Background(), modelConfig{
		PersonaPath: "./personas.json",
		OutputDir:   "./outputs",
		MaxTurns:    8,
		Runner:      &fakeRunner{},
		Loader:      persona.LoadFromFile,
		Now:         time.Now,
	})
	m.personas = []persona.Persona{
		{ID: "p1", Name: "Alpha Persona With Long Name", Role: "very-long-role-name", MasterName: "Very Long Master Name"},
		{ID: "p2", Name: "Beta Persona With Long Name", Role: "another-very-long-role-name"},
	}

	width := 18
	maxLines := 4
	panel := m.buildPersonaPanel(width, maxLines)
	lines := strings.Split(panel, "\n")
	if len(lines) > maxLines {
		t.Fatalf("expected at most %d lines, got %d: %q", maxLines, len(lines), panel)
	}
	for _, line := range lines {
		if lipgloss.Width(line) > width {
			t.Fatalf("expected no wrapped/overflow line (width=%d), got line=%q (w=%d)", width, line, lipgloss.Width(line))
		}
	}
}

func TestBuildPersonaPanelNormalizesMultilineFields(t *testing.T) {
	m := newModel(context.Background(), modelConfig{
		PersonaPath: "./personas.json",
		OutputDir:   "./outputs",
		MaxTurns:    8,
		Runner:      &fakeRunner{},
		Loader:      persona.LoadFromFile,
		Now:         time.Now,
	})
	m.personas = []persona.Persona{
		{
			ID:            "p1",
			Name:          "Alpha\nPersona",
			MasterName:    "Very\nLong Master",
			Role:          "very-long-role\nwith-wrap-risk",
			Stance:        "neutral",
			SignatureLens: []string{"lens line 1\nlens line 2"},
		},
		{ID: "p2", Name: "Beta Persona", Role: "ops"},
	}

	width := 24
	maxLines := 6
	panel := m.buildPersonaPanel(width, maxLines)
	lines := strings.Split(panel, "\n")
	if len(lines) > maxLines {
		t.Fatalf("expected at most %d lines, got %d: %q", maxLines, len(lines), panel)
	}
	for _, line := range lines {
		if strings.ContainsAny(line, "\r\n\t") {
			t.Fatalf("expected sanitized single-line text, got %q", line)
		}
		if lipgloss.Width(line) > width {
			t.Fatalf("expected width <= %d, got line=%q (w=%d)", width, line, lipgloss.Width(line))
		}
	}
}

func TestBuildPersonaPanelRespectsSingleLineLimit(t *testing.T) {
	m := newModel(context.Background(), modelConfig{
		PersonaPath: "./personas.json",
		OutputDir:   "./outputs",
		MaxTurns:    8,
		Runner:      &fakeRunner{},
		Loader:      persona.LoadFromFile,
		Now:         time.Now,
	})
	m.personas = []persona.Persona{
		{ID: "p1", Name: "Alpha", Role: "r1"},
		{ID: "p2", Name: "Beta", Role: "r2"},
		{ID: "p3", Name: "Gamma", Role: "r3"},
	}

	panel := m.buildPersonaPanel(28, 1)
	if len(strings.Split(panel, "\n")) > 1 {
		t.Fatalf("expected single-line panel, got %q", panel)
	}
	if !strings.Contains(panel, "more personas") {
		t.Fatalf("expected overflow summary in single-line panel, got %q", panel)
	}
}

func TestBuildPersonaPanelEmptyStateRespectsSingleLineLimit(t *testing.T) {
	m := newModel(context.Background(), modelConfig{
		PersonaPath: "./personas.json",
		OutputDir:   "./outputs",
		MaxTurns:    8,
		Runner:      &fakeRunner{},
		Loader:      persona.LoadFromFile,
		Now:         time.Now,
	})

	panel := m.buildPersonaPanel(20, 1)
	if len(strings.Split(panel, "\n")) > 1 {
		t.Fatalf("expected single-line empty panel, got %q", panel)
	}
	if !strings.Contains(panel, "no personas loaded") {
		t.Fatalf("expected empty-state message, got %q", panel)
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

func TestViewDoesNotOverflowWindow(t *testing.T) {
	m := newModel(context.Background(), modelConfig{
		PersonaPath: "./personas.json",
		OutputDir:   "./outputs",
		MaxTurns:    8,
		Runner:      &fakeRunner{},
		Loader:      persona.LoadFromFile,
		Now:         time.Now,
	})
	m.personas = []persona.Persona{
		{ID: "p1", Name: "Alpha", Role: "growth"},
		{ID: "p2", Name: "Beta", Role: "risk"},
		{ID: "p3", Name: "Gamma", Role: "data"},
		{ID: "p4", Name: "Delta", Role: "ops"},
	}
	for i := 0; i < 120; i++ {
		m.appendLog("this is a sample long log line for wrapping and viewport checks")
	}

	sizes := []struct {
		w int
		h int
	}{
		{w: 76, h: 18},
		{w: 80, h: 24},
		{w: 100, h: 30},
		{w: 120, h: 36},
		{w: 68, h: 20},
		{w: 52, h: 16},
	}

	for _, size := range sizes {
		m.width = size.w
		m.height = size.h
		m.resizeLayout()

		view := m.View()
		if h := lipgloss.Height(view); h > size.h {
			t.Fatalf("rendered height overflow for %dx%d: got %d", size.w, size.h, h)
		}
		for _, line := range strings.Split(view, "\n") {
			if w := lipgloss.Width(line); w > size.w {
				t.Fatalf("rendered width overflow for %dx%d: got %d, line=%q", size.w, size.h, w, line)
			}
		}
	}
}

func TestPersonaPanelHeightStableWithIdeasPreset(t *testing.T) {
	personas, err := persona.LoadFromFile(filepath.Join("..", "..", "exmaples", "personas.ideas.json"))
	if err != nil {
		t.Fatalf("load personas preset: %v", err)
	}

	m := newModel(context.Background(), modelConfig{
		PersonaPath: "./personas.json",
		OutputDir:   "./outputs",
		MaxTurns:    8,
		Runner:      &fakeRunner{},
		Loader:      persona.LoadFromFile,
		Now:         time.Now,
	})
	m.personas = personas
	m.width = 150
	m.height = 60

	contentWidth := maxInt(1, m.width-viewChromeStyle.GetHorizontalFrameSize())
	hero := m.renderHero(contentWidth)
	commands := m.renderCommandRibbon(contentWidth)
	footer := m.renderFooter(contentWidth)
	availableBodyH := m.height - viewChromeStyle.GetVerticalFrameSize() - lipgloss.Height(hero) - lipgloss.Height(commands) - lipgloss.Height(footer)
	minStandardBodyOuterH := maxInt(6, viewPanelStyle.GetVerticalFrameSize()+4)
	if availableBodyH < minStandardBodyOuterH {
		t.Fatalf("unexpected compact mode in test setup: availableBodyH=%d", availableBodyH)
	}

	leftOuterW := minInt(48, maxInt(32, contentWidth/3))
	panelOuterH := maxInt(minStandardBodyOuterH, availableBodyH)
	panelBoxLeftW := styleBoxWidth(viewPanelStyle, leftOuterW)
	panelTextLeftW := styleTextWidth(viewPanelStyle, leftOuterW)
	panelBoxH := styleBoxHeight(viewPanelStyle, panelOuterH)
	panelTextH := styleTextHeight(viewPanelStyle, panelOuterH)
	leftHeader := m.renderPanelHeader("PERSONAS", "loaded", maxInt(12, panelTextLeftW))
	leftBodyH := maxInt(1, panelTextH-lipgloss.Height(leftHeader))

	body := m.buildPersonaPanel(maxInt(12, panelTextLeftW), maxInt(1, leftBodyH))
	panel := viewPanelStyle.Width(panelBoxLeftW).Height(panelBoxH).Render(lipgloss.JoinVertical(lipgloss.Left, leftHeader, body))
	if got := lipgloss.Height(panel); got != panelOuterH {
		t.Fatalf("persona panel height expanded unexpectedly: got=%d want=%d", got, panelOuterH)
	}

	for _, line := range strings.Split(body, "\n") {
		if lipgloss.Width(line) > panelTextLeftW {
			t.Fatalf("persona line overflow: w=%d limit=%d line=%q", lipgloss.Width(line), panelTextLeftW, line)
		}
	}
}
