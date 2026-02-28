package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"debate/internal/config"
	"debate/internal/openai"
	"debate/internal/orchestrator"
	"debate/internal/persona"
	"debate/internal/repl"
	"debate/internal/tui"
	"golang.org/x/term"
)

func main() {
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
			PersonaPath: config.DefaultPersonaPath,
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
		PersonaPath: config.DefaultPersonaPath,
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
