package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"debate/internal/orchestrator"
	"debate/internal/output"
	"debate/internal/persona"
)

type Runner interface {
	Run(ctx context.Context, problem string, personas []persona.Persona, onTurn func(orchestrator.Turn)) (orchestrator.Result, error)
}

type LoaderFunc func(path string) ([]persona.Persona, error)

type Config struct {
	PersonaPath string
	OutputDir   string
	MaxTurns    int
	Runner      Runner
	Loader      LoaderFunc
	Now         func() time.Time
}

type App struct {
	personaPath string
	outputDir   string
	maxTurns    int
	runner      Runner
	loader      LoaderFunc
	now         func() time.Time
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
		maxTurns:    normalizeMaxTurns(cfg.MaxTurns),
		runner:      cfg.Runner,
		loader:      cfg.Loader,
		now:         cfg.Now,
	}
}

func (a *App) Start(ctx context.Context) error {
	if a.runner == nil {
		return errors.New("runner is required")
	}

	m := newModel(ctx, modelConfig{
		PersonaPath: a.personaPath,
		OutputDir:   a.outputDir,
		MaxTurns:    a.maxTurns,
		Runner:      a.runner,
		Loader:      a.loader,
		Now:         a.now,
	})

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}

type modelConfig struct {
	PersonaPath string
	OutputDir   string
	MaxTurns    int
	Runner      Runner
	Loader      LoaderFunc
	Now         func() time.Time
}

type model struct {
	ctx context.Context

	personaPath string
	outputDir   string
	maxTurns    int
	runner      Runner
	loader      LoaderFunc
	now         func() time.Time

	input            textinput.Model
	logViewport      viewport.Model
	spin             spinner.Model
	logs             []string
	wrappedLogs      []string
	wrappedWidth     int
	width            int
	height           int
	running          bool
	runningSince     time.Time
	totalTurnCount   int
	personaTurnCount int
	speakerTurns     map[string]int
	lastSpeakerName  string
	autoFollow       bool
	debateCancel     context.CancelFunc

	commandHistory []string
	historyCursor  int

	personas       []persona.Persona
	lastResultPath string
}

const (
	defaultWidth  = 100
	defaultHeight = 32
	logBufferMax  = 4000
	scrollStep    = 5
)

type personasLoadedMsg struct {
	personas []persona.Persona
	err      error
}

type debateTurnMsg struct {
	turn orchestrator.Turn
}

type debateStreamStartedMsg struct {
	events <-chan tea.Msg
}

type debateStreamMsg struct {
	events  <-chan tea.Msg
	payload tea.Msg
	closed  bool
}

type debateCompletedMsg struct {
	result  *orchestrator.Result
	path    string
	err     error
	saveErr error
}

func newModel(ctx context.Context, cfg modelConfig) model {
	if cfg.Loader == nil {
		cfg.Loader = persona.LoadFromFile
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}

	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "Ask anything. /ask <topic> or just type a sentence"
	ti.Focus()
	ti.CharLimit = 1024 * 32
	ti.Width = defaultWidth - 4

	vp := viewport.New(defaultWidth-4, defaultHeight-12)
	vp.SetContent("")

	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("45"))

	m := model{
		ctx:           ctx,
		personaPath:   cfg.PersonaPath,
		outputDir:     cfg.OutputDir,
		maxTurns:      normalizeMaxTurns(cfg.MaxTurns),
		runner:        cfg.Runner,
		loader:        cfg.Loader,
		now:           cfg.Now,
		input:         ti,
		logViewport:   vp,
		spin:          sp,
		logs:          []string{"Debate Studio ready."},
		width:         defaultWidth,
		height:        defaultHeight,
		autoFollow:    true,
		speakerTurns:  make(map[string]int),
		historyCursor: 0,
	}
	m.resizeLayout()
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, loadPersonasCmd(m.personaPath, m.loader), m.spin.Tick)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = typed.Width
		m.height = typed.Height
		m.resizeLayout()
		return m, nil

	case spinner.TickMsg:
		return m, m.updateSpinner(typed)

	case tea.KeyMsg:
		if cmd, handled := m.handleKeyMessage(typed); handled {
			return m, cmd
		}

	case personasLoadedMsg:
		m.handlePersonasLoaded(typed)
		return m, nil

	case debateStreamStartedMsg:
		return m, listenDebateEventsCmd(typed.events)

	case debateStreamMsg:
		return m.handleDebateStreamMessage(typed)

	case debateCompletedMsg:
		// Backward compatibility: treat direct completion as final event.
		m.applyDebateCompleted(typed)
		return m, nil
	}

	return m, m.updateInteractiveInputs(msg)
}

func (m *model) updateSpinner(msg spinner.TickMsg) tea.Cmd {
	var cmd tea.Cmd
	m.spin, cmd = m.spin.Update(msg)
	if m.running {
		return cmd
	}
	return nil
}

func (m *model) handleKeyMessage(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyCtrlC:
		if m.debateCancel != nil {
			m.debateCancel()
			m.debateCancel = nil
		}
		return tea.Quit, true
	case tea.KeyCtrlF:
		m.autoFollow = !m.autoFollow
		if m.autoFollow {
			m.logViewport.GotoBottom()
		}
		m.appendLog(fmt.Sprintf("auto-follow: %s", onOff(m.autoFollow)))
		return nil, true
	case tea.KeyCtrlL:
		m.logs = nil
		m.refreshLogViewport()
		return nil, true
	case tea.KeyCtrlP:
		m.input.SetValue(m.historyPrev())
		m.input.CursorEnd()
		return nil, true
	case tea.KeyCtrlN:
		m.input.SetValue(m.historyNext())
		m.input.CursorEnd()
		return nil, true
	case tea.KeyPgUp:
		m.autoFollow = false
		m.logViewport.LineUp(scrollStep)
		return nil, true
	case tea.KeyPgDown:
		m.autoFollow = false
		m.logViewport.LineDown(scrollStep)
		return nil, true
	case tea.KeyHome:
		m.autoFollow = false
		m.logViewport.GotoTop()
		return nil, true
	case tea.KeyEnd:
		m.autoFollow = true
		m.logViewport.GotoBottom()
		return nil, true
	case tea.KeyEnter:
		cmdLine := strings.TrimSpace(m.input.Value())
		m.input.SetValue("")
		if cmdLine == "" {
			return nil, true
		}
		m.pushHistory(cmdLine)
		return m.handleCommand(cmdLine), true
	default:
		return nil, false
	}
}

func (m *model) handlePersonasLoaded(msg personasLoadedMsg) {
	if msg.err != nil {
		m.appendLog(fmt.Sprintf("Failed to load %s: %v", m.personaPath, msg.err))
		m.appendLog("Use /load after fixing the file.")
		return
	}
	m.personas = msg.personas
	m.appendLog(fmt.Sprintf("Loaded %d personas from %s", len(msg.personas), m.personaPath))
}

func (m *model) handleDebateStreamMessage(msg debateStreamMsg) (tea.Model, tea.Cmd) {
	if msg.closed {
		if m.running {
			m.running = false
			m.debateCancel = nil
			m.appendLogs("debate stream closed", "==== debate end ====")
		}
		return *m, nil
	}

	switch payload := msg.payload.(type) {
	case debateTurnMsg:
		m.totalTurnCount++
		if payload.turn.Type == orchestrator.TurnTypePersona {
			m.personaTurnCount++
		}
		m.speakerTurns[payload.turn.SpeakerID]++
		m.lastSpeakerName = payload.turn.SpeakerName
		m.appendTurnLog(payload.turn)
		return *m, listenDebateEventsCmd(msg.events)
	case debateCompletedMsg:
		m.applyDebateCompleted(payload)
		return *m, nil
	default:
		return *m, listenDebateEventsCmd(msg.events)
	}
}

func (m *model) applyDebateCompleted(msg debateCompletedMsg) {
	m.running = false
	m.debateCancel = nil
	if msg.err != nil {
		m.appendLog(fmt.Sprintf("debate failed: %v", msg.err))
		m.appendLog("==== debate end ====")
		return
	}
	if msg.saveErr != nil {
		m.appendLog(fmt.Sprintf("save failed: %v", msg.saveErr))
	} else {
		m.lastResultPath = msg.path
		m.appendLog("saved result: " + msg.path)
		m.appendLog("saved markdown: " + output.MarkdownPath(msg.path))
	}
	if msg.result != nil {
		m.appendLog("status: " + msg.result.Status)
		m.appendLog(fmt.Sprintf("consensus score: %.2f", msg.result.Consensus.Score))
		m.appendLog("summary: " + msg.result.Consensus.Summary)
	}
	m.appendLog("==== debate end ====")
}

func (m *model) updateInteractiveInputs(msg tea.Msg) tea.Cmd {
	mouseWheelUp, mouseWheelDown := isMouseWheelScroll(msg)
	var viewportCmd tea.Cmd
	var inputCmd tea.Cmd
	m.logViewport, viewportCmd = m.logViewport.Update(msg)
	m.input, inputCmd = m.input.Update(msg)
	if mouseWheelUp {
		m.autoFollow = false
	}
	if mouseWheelDown && m.logViewport.AtBottom() {
		m.autoFollow = true
	}
	return tea.Batch(viewportCmd, inputCmd)
}

func isMouseWheelScroll(msg tea.Msg) (up bool, down bool) {
	mm, ok := msg.(tea.MouseMsg)
	if !ok || mm.Action != tea.MouseActionPress {
		return false, false
	}
	switch mm.Button { //nolint:exhaustive
	case tea.MouseButtonWheelUp:
		return true, false
	case tea.MouseButtonWheelDown:
		return false, true
	default:
		return false, false
	}
}
