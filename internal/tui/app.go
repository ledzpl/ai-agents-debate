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
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "명령 입력 (/ask 질문, /show, /load, /exit | 일반 문장 입력 가능)"
	ti.Focus()
	ti.CharLimit = 1024 * 32
	ti.Width = defaultWidth - 4

	vp := viewport.New(defaultWidth-4, defaultHeight-12)
	vp.SetContent("")

	sp := spinner.New()
	sp.Spinner = spinner.Line
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))

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
		logs:          []string{"Debate TUI initialized."},
		width:         defaultWidth,
		height:        defaultHeight,
		autoFollow:    true,
		historyCursor: 0,
	}
	m.refreshLogViewport()
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, loadPersonasCmd(m.personaPath, m.loader), m.spin.Tick)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeLayout()
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		if m.running {
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.debateCancel != nil {
				m.debateCancel()
				m.debateCancel = nil
			}
			return m, tea.Quit
		case tea.KeyCtrlF:
			m.autoFollow = !m.autoFollow
			if m.autoFollow {
				m.logViewport.GotoBottom()
				m.appendLog("auto-follow: ON")
			} else {
				m.appendLog("auto-follow: OFF")
			}
			return m, nil
		case tea.KeyCtrlL:
			m.logs = nil
			m.refreshLogViewport()
			return m, nil
		case tea.KeyCtrlP:
			m.input.SetValue(m.historyPrev())
			m.input.CursorEnd()
			return m, nil
		case tea.KeyCtrlN:
			m.input.SetValue(m.historyNext())
			m.input.CursorEnd()
			return m, nil
		case tea.KeyPgUp:
			m.autoFollow = false
			m.logViewport.LineUp(scrollStep)
			return m, nil
		case tea.KeyPgDown:
			m.autoFollow = false
			m.logViewport.LineDown(scrollStep)
			return m, nil
		case tea.KeyHome:
			m.autoFollow = false
			m.logViewport.GotoTop()
			return m, nil
		case tea.KeyEnd:
			m.autoFollow = true
			m.logViewport.GotoBottom()
			return m, nil
		case tea.KeyEnter:
			cmdLine := strings.TrimSpace(m.input.Value())
			m.input.SetValue("")
			if cmdLine == "" {
				return m, nil
			}
			m.pushHistory(cmdLine)
			return m, m.handleCommand(cmdLine)
		}

	case personasLoadedMsg:
		if msg.err != nil {
			m.appendLog(fmt.Sprintf("Failed to load %s: %v", m.personaPath, msg.err))
			m.appendLog("Use /load after fixing the file.")
			return m, nil
		}
		m.personas = msg.personas
		m.appendLog(fmt.Sprintf("Loaded %d personas from %s", len(msg.personas), m.personaPath))
		return m, nil

	case debateStreamStartedMsg:
		return m, listenDebateEventsCmd(msg.events)

	case debateStreamMsg:
		if msg.closed {
			if m.running {
				m.running = false
				m.debateCancel = nil
				m.appendLogs("debate stream closed", "==== debate end ====")
			}
			return m, nil
		}
		switch payload := msg.payload.(type) {
		case debateTurnMsg:
			m.totalTurnCount++
			if payload.turn.Type == orchestrator.TurnTypePersona {
				m.personaTurnCount++
			}
			m.appendTurnLog(payload.turn)
			return m, listenDebateEventsCmd(msg.events)
		case debateCompletedMsg:
			m.running = false
			m.debateCancel = nil
			if payload.err != nil {
				m.appendLog(fmt.Sprintf("debate failed: %v", payload.err))
				m.appendLog("==== debate end ====")
				return m, nil
			}
			if payload.saveErr != nil {
				m.appendLog(fmt.Sprintf("save failed: %v", payload.saveErr))
			} else {
				m.lastResultPath = payload.path
				m.appendLog("saved result: " + payload.path)
			}

			if payload.result != nil {
				m.appendLog("status: " + payload.result.Status)
				m.appendLog(fmt.Sprintf("consensus score: %.2f", payload.result.Consensus.Score))
				m.appendLog("summary: " + payload.result.Consensus.Summary)
			}
			m.appendLog("==== debate end ====")
			return m, nil
		default:
			return m, listenDebateEventsCmd(msg.events)
		}

	case debateCompletedMsg:
		// Backward compatibility: treat direct completion as final event.
		m.running = false
		m.debateCancel = nil
		if msg.err != nil {
			m.appendLog(fmt.Sprintf("debate failed: %v", msg.err))
			m.appendLog("==== debate end ====")
			return m, nil
		}
		if msg.saveErr != nil {
			m.appendLog(fmt.Sprintf("save failed: %v", msg.saveErr))
		} else {
			m.lastResultPath = msg.path
			m.appendLog("saved result: " + msg.path)
		}
		if msg.result != nil {
			m.appendLog("status: " + msg.result.Status)
			m.appendLog(fmt.Sprintf("consensus score: %.2f", msg.result.Consensus.Score))
			m.appendLog("summary: " + msg.result.Consensus.Summary)
		}
		m.appendLog("==== debate end ====")
		return m, nil
	}

	var cmds []tea.Cmd
	var inputCmd tea.Cmd
	var viewportCmd tea.Cmd
	mouseWheelUp := false
	mouseWheelDown := false
	if mm, ok := msg.(tea.MouseMsg); ok && mm.Action == tea.MouseActionPress {
		switch mm.Button { //nolint:exhaustive
		case tea.MouseButtonWheelUp:
			mouseWheelUp = true
		case tea.MouseButtonWheelDown:
			mouseWheelDown = true
		}
	}

	m.logViewport, viewportCmd = m.logViewport.Update(msg)
	m.input, inputCmd = m.input.Update(msg)
	if mouseWheelUp {
		m.autoFollow = false
	}
	if mouseWheelDown && m.logViewport.AtBottom() {
		m.autoFollow = true
	}
	cmds = append(cmds, viewportCmd, inputCmd)

	return m, tea.Batch(cmds...)
}
