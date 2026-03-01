package repl

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"debate/internal/commandutil"
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
	Runner      Runner
	Loader      LoaderFunc
	Writer      io.Writer
	Now         func() time.Time
}

type App struct {
	personaPath string
	outputDir   string
	runner      Runner
	loader      LoaderFunc
	writer      io.Writer
	now         func() time.Time

	personas       []persona.Persona
	lastResultPath string
}

const maxREPLInputBytes = 1024 * 1024

var replCommandAliases = map[string]string{
	"ask":   "/ask",
	"/ask":  "/ask",
	"show":  "/show",
	"/show": "/show",
	"load":  "/load",
	"/load": "/load",
	"help":  "/help",
	"/help": "/help",
	"exit":  "/exit",
	"/exit": "/exit",
}

func NewApp(cfg Config) *App {
	if cfg.Loader == nil {
		cfg.Loader = persona.LoadFromFile
	}
	if cfg.Writer == nil {
		cfg.Writer = io.Discard
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &App{
		personaPath: cfg.PersonaPath,
		outputDir:   cfg.OutputDir,
		runner:      cfg.Runner,
		loader:      cfg.Loader,
		writer:      cfg.Writer,
		now:         cfg.Now,
	}
}

func (a *App) Start(ctx context.Context, in io.Reader) error {
	if a.runner == nil {
		return errors.New("runner is required")
	}
	if in == nil {
		return errors.New("input reader is required")
	}

	a.printLine("Multi-persona Debate REPL")
	a.printLine("Commands: /load, /show, /ask <problem>, /help, /exit")

	if err := a.loadPersonas(); err != nil {
		a.printLine(fmt.Sprintf("Failed to load %s: %v", a.personaPath, err))
		a.printLine("Use /load after fixing the file.")
	} else {
		a.printLine(fmt.Sprintf("Loaded %d personas from %s", len(a.personas), a.personaPath))
	}

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), maxREPLInputBytes)
	for {
		if _, err := fmt.Fprint(a.writer, "debate> "); err != nil {
			return err
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			a.printLine("")
			return nil
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		quit := a.handleLine(ctx, line)
		if quit {
			return nil
		}
	}
}

func (a *App) handleLine(ctx context.Context, line string) bool {
	command, arg := parseCommand(line)
	switch command {
	case "/exit":
		a.printLine("bye")
		return true
	case "/help":
		if arg != "" {
			a.printLine("usage: /help")
			return false
		}
		a.printHelp()
		return false
	case "/load":
		if arg != "" {
			a.printLine("usage: /load")
			return false
		}
		if err := a.loadPersonas(); err != nil {
			a.printLine(fmt.Sprintf("load failed: %v", err))
		} else {
			a.printLine(fmt.Sprintf("loaded %d personas", len(a.personas)))
		}
		return false
	case "/show":
		if arg != "" {
			a.printLine("usage: /show")
			return false
		}
		a.showPersonas()
		return false
	case "/ask":
		if arg == "" {
			a.printLine("usage: /ask <problem>")
			return false
		}
		a.runDebate(ctx, arg)
		return false
	default:
		if strings.HasPrefix(strings.TrimSpace(line), "/") {
			a.printLine("unknown command. Use /load, /show, /ask <problem>, /help, /exit")
			return false
		}
		_ = a.handleLine(ctx, "/ask "+strings.TrimSpace(line))
		return false
	}
}

func (a *App) loadPersonas() error {
	personas, err := a.loader(a.personaPath)
	if err != nil {
		return err
	}
	a.personas = personas
	return nil
}

func (a *App) showPersonas() {
	if len(a.personas) == 0 {
		a.printLine("no personas loaded")
		return
	}

	a.printLine(fmt.Sprintf("personas (%d):", len(a.personas)))
	for i, p := range a.personas {
		a.printLine(fmt.Sprintf("%d. %s (%s) role=%s stance=%s", i+1, persona.DisplayName(p), p.ID, p.Role, p.Stance))
	}
	if a.lastResultPath != "" {
		a.printLine("last result: " + a.lastResultPath)
	}
}

func (a *App) runDebate(ctx context.Context, problem string) {
	if len(a.personas) == 0 {
		a.printLine("no personas loaded; use /load")
		return
	}

	a.printLine("running debate...")
	result, err := a.runner.Run(ctx, problem, a.personas, func(turn orchestrator.Turn) {
		for _, line := range formatTurnLines(turn) {
			a.printLine(line)
		}
	})
	if err != nil {
		a.printLine(fmt.Sprintf("debate failed: %v", err))
		return
	}

	path := output.NewTimestampPath(a.outputDir, a.now())
	if err := output.SaveResult(path, result); err != nil {
		a.printLine(fmt.Sprintf("save failed: %v", err))
	} else {
		a.lastResultPath = path
		a.printLine("saved result: " + path)
		a.printLine("saved markdown: " + output.MarkdownPath(path))
	}

	a.printLine("status: " + result.Status)
	a.printLine(fmt.Sprintf("consensus score: %.2f", result.Consensus.Score))
	a.printLine("summary: " + result.Consensus.Summary)
}

func (a *App) printLine(msg string) {
	_, _ = fmt.Fprintln(a.writer, msg)
}

func parseCommand(line string) (command string, arg string) {
	return commandutil.Parse(line, replCommandAliases)
}

func formatTurnLines(turn orchestrator.Turn) []string {
	label := turn.SpeakerName
	if turn.Type == orchestrator.TurnTypeModerator {
		label = "사회자"
	}

	separator := strings.Repeat("-", 52)
	if turn.Type == orchestrator.TurnTypeModerator {
		separator = strings.Repeat("=", 52)
	}

	header := fmt.Sprintf("---- turn %d | %s ----", turn.Index, label)
	lines := []string{"", separator, header}

	contentLines := strings.Split(strings.TrimSpace(turn.Content), "\n")
	appended := false
	for _, line := range contentLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lines = append(lines, "  "+trimmed)
		appended = true
	}
	if !appended {
		lines = append(lines, "  (empty)")
	}
	lines = append(lines, separator, "")

	return lines
}

func (a *App) printHelp() {
	a.printLine("commands:")
	a.printLine("  /ask <problem>  : start a debate")
	a.printLine("  /show           : show loaded personas")
	a.printLine("  /load           : reload personas.json")
	a.printLine("  /help           : show this help")
	a.printLine("  /exit           : quit")
}
