package main

import (
	"testing"
	"time"

	"debate/internal/config"
)

func TestParseRuntimeOptionsDefaults(t *testing.T) {
	opts, err := parseRuntimeOptions(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.personaPath != config.DefaultPersonaPath {
		t.Fatalf("unexpected default persona path: %s", opts.personaPath)
	}
	if opts.addr != "" {
		t.Fatalf("expected empty addr by default, got %q", opts.addr)
	}
}

func TestParseRuntimeOptionsPersonasFlag(t *testing.T) {
	opts, err := parseRuntimeOptions([]string{"--personas", "./exmaples/personas.pm.json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.personaPath != "./exmaples/personas.pm.json" {
		t.Fatalf("unexpected persona path: %s", opts.personaPath)
	}
}

func TestParseRuntimeOptionsPersonaAlias(t *testing.T) {
	opts, err := parseRuntimeOptions([]string{"--persona", "./custom.json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.personaPath != "./custom.json" {
		t.Fatalf("unexpected persona path: %s", opts.personaPath)
	}
}

func TestParseRuntimeOptionsRejectsPositionalArgs(t *testing.T) {
	_, err := parseRuntimeOptions([]string{"unexpected"})
	if err == nil {
		t.Fatal("expected error for positional args")
	}
}

func TestParseRuntimeOptionsAddr(t *testing.T) {
	opts, err := parseRuntimeOptions([]string{"--addr", "  :8090  "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.addr != ":8090" {
		t.Fatalf("unexpected addr: %q", opts.addr)
	}
}

func TestParseRuntimeOptionsRejectsUnknownFlag(t *testing.T) {
	_, err := parseRuntimeOptions([]string{"--web"})
	if err == nil {
		t.Fatal("expected unknown flag error")
	}
}

func TestOrchestratorConfigFromSettings(t *testing.T) {
	settings := config.Settings{
		MaxTurns:           11,
		ConsensusThreshold: 0.88,
		MaxDuration:        17 * time.Minute,
		MaxTotalTokens:     45000,
		MaxNoProgressJudge: 4,
		HardMaxTurns:       320,
		DirectJudgeEvery:   3,
		LLMHistoryWindow:   90,
		AudienceMode:       "expert",
	}

	got := orchestratorConfigFromSettings(settings)
	if got.MaxTurns != settings.MaxTurns {
		t.Fatalf("unexpected max turns: %d", got.MaxTurns)
	}
	if got.ConsensusThreshold != settings.ConsensusThreshold {
		t.Fatalf("unexpected threshold: %v", got.ConsensusThreshold)
	}
	if got.MaxDuration != settings.MaxDuration {
		t.Fatalf("unexpected duration: %s", got.MaxDuration)
	}
	if got.MaxTotalTokens != settings.MaxTotalTokens {
		t.Fatalf("unexpected max total tokens: %d", got.MaxTotalTokens)
	}
	if got.MaxNoProgressJudges != settings.MaxNoProgressJudge {
		t.Fatalf("unexpected max no progress judges: %d", got.MaxNoProgressJudges)
	}
	if got.UnlimitedHardMaxTurns != settings.HardMaxTurns {
		t.Fatalf("unexpected hard max turns: %d", got.UnlimitedHardMaxTurns)
	}
	if got.DirectHandoffJudgeEvery != settings.DirectJudgeEvery {
		t.Fatalf("unexpected direct judge cadence: %d", got.DirectHandoffJudgeEvery)
	}
	if got.LLMHistoryTurnWindow != settings.LLMHistoryWindow {
		t.Fatalf("unexpected history window: %d", got.LLMHistoryTurnWindow)
	}
	if got.AudienceMode != settings.AudienceMode {
		t.Fatalf("unexpected audience mode: %s", got.AudienceMode)
	}
}
