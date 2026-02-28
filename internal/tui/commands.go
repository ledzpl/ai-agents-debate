package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"debate/internal/orchestrator"
	"debate/internal/output"
	"debate/internal/persona"
)

func (m *model) handleCommand(line string) tea.Cmd {
	command, arg := parseCommand(line)
	switch command {
	case "/exit":
		return m.handleExitCommand()
	case "/stop":
		return m.handleStopCommand(arg)
	case "/follow":
		return m.handleFollowCommand(arg)
	case "/help":
		return m.handleHelpCommand(arg)
	case "/load":
		return m.handleLoadCommand(arg)
	case "/show":
		return m.handleShowCommand(arg)
	case "/ask":
		return m.handleAskCommand(arg)
	default:
		return m.handleUnknownOrPlainText(line)
	}
}

func (m *model) handleExitCommand() tea.Cmd {
	if m.debateCancel != nil {
		m.debateCancel()
		m.debateCancel = nil
	}
	m.appendLog("bye")
	return tea.Quit
}

func (m *model) handleStopCommand(arg string) tea.Cmd {
	if arg != "" {
		m.appendLog("usage: /stop")
		return nil
	}
	if !m.running || m.debateCancel == nil {
		m.appendLog("no running debate to stop")
		return nil
	}
	m.appendLog("stop requested...")
	m.debateCancel()
	return nil
}

func (m *model) handleFollowCommand(arg string) tea.Cmd {
	mode := strings.ToLower(strings.TrimSpace(arg))
	if mode == "" || mode == "toggle" {
		m.autoFollow = !m.autoFollow
		if m.autoFollow {
			m.logViewport.GotoBottom()
		}
		m.appendLog(fmt.Sprintf("auto-follow: %s", onOff(m.autoFollow)))
		return nil
	}

	switch mode {
	case "on":
		m.autoFollow = true
		m.logViewport.GotoBottom()
		m.appendLog("auto-follow: ON")
	case "off":
		m.autoFollow = false
		m.appendLog("auto-follow: OFF")
	default:
		m.appendLog("usage: /follow [on|off|toggle]")
	}
	return nil
}

func (m *model) handleHelpCommand(arg string) tea.Cmd {
	if arg != "" {
		m.appendLog("usage: /help")
		return nil
	}
	m.appendHelp()
	return nil
}

func (m *model) handleLoadCommand(arg string) tea.Cmd {
	if arg != "" {
		m.appendLog("usage: /load")
		return nil
	}
	return loadPersonasCmd(m.personaPath, m.loader)
}

func (m *model) handleShowCommand(arg string) tea.Cmd {
	if arg != "" {
		m.appendLog("usage: /show")
		return nil
	}
	m.appendPersonaList()
	return nil
}

func (m *model) handleAskCommand(arg string) tea.Cmd {
	if arg == "" {
		m.appendLog("usage: /ask <problem>")
		return nil
	}
	if m.running {
		m.appendLog("a debate is already running")
		return nil
	}
	if len(m.personas) == 0 {
		m.appendLog("no personas loaded; use /load")
		return nil
	}
	return m.startDebate(arg)
}

func (m *model) startDebate(problem string) tea.Cmd {
	m.running = true
	m.autoFollow = true
	m.runningSince = m.now()
	m.totalTurnCount = 0
	m.personaTurnCount = 0
	m.speakerTurns = make(map[string]int)
	m.lastSpeakerName = ""

	runCtx, cancel := context.WithCancel(m.ctx)
	m.debateCancel = cancel

	m.appendLog("==== debate start ====")
	m.appendLog("running debate: " + problem)
	return tea.Batch(
		runDebateCmd(runCtx, m.runner, problem, m.personas, m.outputDir, m.now),
		m.spin.Tick,
	)
}

func (m *model) handleUnknownOrPlainText(line string) tea.Cmd {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "/") {
		m.appendLog("unknown command. Use /ask <problem>, /stop, /follow, /show, /load, /help, /exit")
		return nil
	}
	// Plain text is treated as a debate prompt.
	return m.handleCommand("/ask " + trimmed)
}

func (m *model) appendPersonaList() {
	if len(m.personas) == 0 {
		m.appendLog("no personas loaded")
		return
	}
	lines := make([]string, 0, len(m.personas)+1)
	lines = append(lines, fmt.Sprintf("personas (%d):", len(m.personas)))
	for i, p := range m.personas {
		lines = append(lines, fmt.Sprintf("%d. %s (%s) role=%s stance=%s", i+1, persona.DisplayName(p), p.ID, p.Role, p.Stance))
	}
	m.appendLogs(lines...)
}

func (m *model) appendLog(line string) {
	m.appendLogs(line)
}

func (m *model) appendLogs(lines ...string) {
	if len(lines) == 0 {
		return
	}
	m.logs = append(m.logs, lines...)

	trimmed := false
	if len(m.logs) > logBufferMax {
		m.logs = m.logs[len(m.logs)-logBufferMax:]
		trimmed = true
	}

	if trimmed || m.wrappedLogs == nil || m.wrappedWidth != m.logViewport.Width {
		m.refreshLogViewport()
		return
	}

	m.wrappedLogs = append(m.wrappedLogs, wrapLogLines(lines, m.logViewport.Width)...)
	m.logViewport.SetContent(strings.Join(m.wrappedLogs, "\n"))
	if m.autoFollow {
		m.logViewport.GotoBottom()
	}
}

func (m *model) appendTurnLog(turn orchestrator.Turn) {
	m.appendLogs(formatTurnLines(turn)...)
}

func (m *model) appendHelp() {
	m.appendLogs(
		"commands:",
		"  /ask <problem>  : start a debate",
		"  /stop           : stop the running debate",
		"  /follow [mode]  : auto-follow log (on/off/toggle)",
		"  /show           : show loaded personas",
		"  /load           : reload personas.json",
		"  /help           : show this help",
		"  /exit           : quit",
		"shortcuts: Ctrl+P/Ctrl+N history, Ctrl+F follow toggle, PgUp/PgDn/Home/End scroll, wheel/trackpad scroll, Ctrl+L clear",
	)
}

func (m *model) pushHistory(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	if len(m.commandHistory) == 0 || m.commandHistory[len(m.commandHistory)-1] != line {
		m.commandHistory = append(m.commandHistory, line)
	}
	m.historyCursor = len(m.commandHistory)
}

func (m *model) historyPrev() string {
	if len(m.commandHistory) == 0 {
		return ""
	}
	if m.historyCursor > 0 {
		m.historyCursor--
	}
	return m.commandHistory[m.historyCursor]
}

func (m *model) historyNext() string {
	if len(m.commandHistory) == 0 {
		return ""
	}
	if m.historyCursor < len(m.commandHistory)-1 {
		m.historyCursor++
		return m.commandHistory[m.historyCursor]
	}
	m.historyCursor = len(m.commandHistory)
	return ""
}

func loadPersonasCmd(path string, loader LoaderFunc) tea.Cmd {
	return func() tea.Msg {
		personas, err := loader(path)
		return personasLoadedMsg{personas: personas, err: err}
	}
}

func runDebateCmd(ctx context.Context, runner Runner, problem string, personas []persona.Persona, outputDir string, now func() time.Time) tea.Cmd {
	frozenPersonas := append([]persona.Persona(nil), personas...)
	return func() tea.Msg {
		events := make(chan tea.Msg, 64)
		go func() {
			defer close(events)
			send := func(msg tea.Msg) bool {
				select {
				case events <- msg:
					return true
				case <-ctx.Done():
					return false
				}
			}

			result, err := runner.Run(ctx, problem, frozenPersonas, func(turn orchestrator.Turn) {
				_ = send(debateTurnMsg{turn: turn})
			})
			if err != nil {
				_ = send(debateCompletedMsg{err: err})
				return
			}

			path := output.NewTimestampPath(outputDir, now())
			saveErr := output.SaveResult(path, result)
			_ = send(debateCompletedMsg{
				result:  &result,
				path:    path,
				saveErr: saveErr,
			})
		}()

		return debateStreamStartedMsg{events: events}
	}
}

func listenDebateEventsCmd(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			return debateStreamMsg{closed: true}
		}
		return debateStreamMsg{
			events:  events,
			payload: msg,
		}
	}
}
