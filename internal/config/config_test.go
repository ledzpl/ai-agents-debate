package config

import (
	"strings"
	"testing"
	"time"
)

func TestFromEnvMissingAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error when api key is missing")
	}
}

func TestFromEnvSuccess(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", "https://example.com")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIKey != "test-key" {
		t.Fatalf("unexpected api key: %s", cfg.APIKey)
	}
	if cfg.BaseURL != "https://example.com" {
		t.Fatalf("unexpected base url: %s", cfg.BaseURL)
	}
}

func TestFromEnvOverrides(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_MODEL", "gpt-5-mini")
	t.Setenv("DEBATE_MAX_TURNS", "9")
	t.Setenv("DEBATE_CONSENSUS_THRESHOLD", "0.9")
	t.Setenv("DEBATE_MAX_DURATION", "15m")
	t.Setenv("DEBATE_MAX_TOTAL_TOKENS", "32100")
	t.Setenv("DEBATE_MAX_NO_PROGRESS_JUDGE", "4")
	t.Setenv("OPENAI_REQUEST_TIMEOUT", "90s")
	t.Setenv("OPENAI_API_MAX_RETRIES", "5")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Model != "gpt-5-mini" {
		t.Fatalf("unexpected model: %s", cfg.Model)
	}
	if cfg.MaxTurns != 9 {
		t.Fatalf("unexpected max turns: %d", cfg.MaxTurns)
	}
	if cfg.ConsensusThreshold != 0.9 {
		t.Fatalf("unexpected threshold: %v", cfg.ConsensusThreshold)
	}
	if cfg.MaxDuration != 15*time.Minute {
		t.Fatalf("unexpected max duration: %s", cfg.MaxDuration)
	}
	if cfg.MaxTotalTokens != 32100 {
		t.Fatalf("unexpected max total tokens: %d", cfg.MaxTotalTokens)
	}
	if cfg.MaxNoProgressJudge != 4 {
		t.Fatalf("unexpected max no progress judge: %d", cfg.MaxNoProgressJudge)
	}
	if cfg.RequestTimeout != 90*time.Second {
		t.Fatalf("unexpected request timeout: %s", cfg.RequestTimeout)
	}
	if cfg.APIMaxRetries != 5 {
		t.Fatalf("unexpected retries: %d", cfg.APIMaxRetries)
	}
}

func TestFromEnvInvalidOverride(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("DEBATE_CONSENSUS_THRESHOLD", "1.7")

	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "DEBATE_CONSENSUS_THRESHOLD") {
		t.Fatalf("unexpected error: %v", err)
	}
}
