package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"debate/internal/config"
	"debate/internal/openai"
	"debate/internal/orchestrator"
	"debate/internal/persona"
	"debate/internal/repl"
	"debate/internal/tui"
	"golang.org/x/term"
)

type runtimeOptions struct {
	personaPath string
}

func main() {
	opts, err := parseRuntimeOptions(os.Args[1:])
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "argument error:", err)
		os.Exit(1)
	}

	settings, err := config.FromEnv()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}

	client, err := openai.NewClient(openai.Config{
		APIKey:     settings.APIKey,
		BaseURL:    settings.BaseURL,
		Model:      settings.Model,
		Timeout:    settings.RequestTimeout,
		MaxRetries: settings.APIMaxRetries,
	})
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "openai client error:", err)
		os.Exit(1)
	}

	runner := orchestrator.New(client, orchestrator.Config{
		MaxTurns:            settings.MaxTurns,
		ConsensusThreshold:  settings.ConsensusThreshold,
		MaxDuration:         settings.MaxDuration,
		MaxTotalTokens:      settings.MaxTotalTokens,
		MaxNoProgressJudges: settings.MaxNoProgressJudge,
	})

	if isTTY() {
		app := tui.NewApp(tui.Config{
			PersonaPath: opts.personaPath,
			OutputDir:   config.DefaultOutputDir,
			MaxTurns:    settings.MaxTurns,
			Runner:      runner,
			Loader:      persona.LoadFromFile,
			Now:         time.Now,
		})
		if err := app.Start(context.Background()); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "runtime error:", err)
			os.Exit(1)
		}
		return
	}

	// Fallback for non-interactive shells (pipes, CI).
	app := repl.NewApp(repl.Config{
		PersonaPath: opts.personaPath,
		OutputDir:   config.DefaultOutputDir,
		Runner:      runner,
		Loader:      persona.LoadFromFile,
		Writer:      os.Stdout,
		Now:         time.Now,
	})

	if err := app.Start(context.Background(), os.Stdin); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "runtime error:", err)
		os.Exit(1)
	}
}

func isTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

func parseRuntimeOptions(args []string) (runtimeOptions, error) {
	fs := flag.NewFlagSet("debate", flag.ContinueOnError)
	personaPath := fs.String("personas", config.DefaultPersonaPath, "path to personas json file")
	fs.StringVar(personaPath, "persona", config.DefaultPersonaPath, "alias of -personas")
	fs.SetOutput(os.Stderr)

	if err := fs.Parse(args); err != nil {
		return runtimeOptions{}, err
	}
	if len(fs.Args()) > 0 {
		return runtimeOptions{}, fmt.Errorf("unexpected positional args: %s", strings.Join(fs.Args(), " "))
	}

	path := strings.TrimSpace(*personaPath)
	if path == "" {
		path = config.DefaultPersonaPath
	}
	return runtimeOptions{personaPath: path}, nil
}
