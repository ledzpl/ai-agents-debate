package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultPersonaPath        = "./personas.json"
	DefaultOutputDir          = "./outputs"
	DefaultModel              = "gpt-5.2"
	DefaultMaxTurns           = 0
	DefaultConsensusThreshold = 0.90
	DefaultMaxDuration        = 20 * time.Minute
	DefaultMaxTotalTokens     = 120000
	DefaultMaxNoProgressJudge = 6
	DefaultRequestTimeout     = 60 * time.Second
	DefaultAPIMaxRetries      = 2
)

type Settings struct {
	APIKey             string
	BaseURL            string
	Model              string
	MaxTurns           int
	ConsensusThreshold float64
	MaxDuration        time.Duration
	MaxTotalTokens     int
	MaxNoProgressJudge int
	RequestTimeout     time.Duration
	APIMaxRetries      int
}

func FromEnv() (Settings, error) {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return Settings{}, errors.New("OPENAI_API_KEY is required")
	}

	settings := Settings{
		APIKey:             apiKey,
		BaseURL:            strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")),
		Model:              DefaultModel,
		MaxTurns:           DefaultMaxTurns,
		ConsensusThreshold: DefaultConsensusThreshold,
		MaxDuration:        DefaultMaxDuration,
		MaxTotalTokens:     DefaultMaxTotalTokens,
		MaxNoProgressJudge: DefaultMaxNoProgressJudge,
		RequestTimeout:     DefaultRequestTimeout,
		APIMaxRetries:      DefaultAPIMaxRetries,
	}

	if v := strings.TrimSpace(os.Getenv("OPENAI_MODEL")); v != "" {
		settings.Model = v
	}

	var err error
	settings.MaxTurns, err = parseOptionalInt("DEBATE_MAX_TURNS", settings.MaxTurns, func(v int) bool { return v >= 0 })
	if err != nil {
		return Settings{}, err
	}
	settings.ConsensusThreshold, err = parseOptionalFloat64("DEBATE_CONSENSUS_THRESHOLD", settings.ConsensusThreshold, func(v float64) bool { return v >= 0 && v <= 1 })
	if err != nil {
		return Settings{}, err
	}
	settings.MaxDuration, err = parseOptionalDuration("DEBATE_MAX_DURATION", settings.MaxDuration, func(v time.Duration) bool { return v > 0 })
	if err != nil {
		return Settings{}, err
	}
	settings.MaxTotalTokens, err = parseOptionalInt("DEBATE_MAX_TOTAL_TOKENS", settings.MaxTotalTokens, func(v int) bool { return v > 0 })
	if err != nil {
		return Settings{}, err
	}
	settings.MaxNoProgressJudge, err = parseOptionalInt("DEBATE_MAX_NO_PROGRESS_JUDGE", settings.MaxNoProgressJudge, func(v int) bool { return v > 0 })
	if err != nil {
		return Settings{}, err
	}
	settings.RequestTimeout, err = parseOptionalDuration("OPENAI_REQUEST_TIMEOUT", settings.RequestTimeout, func(v time.Duration) bool { return v > 0 })
	if err != nil {
		return Settings{}, err
	}
	settings.APIMaxRetries, err = parseOptionalInt("OPENAI_API_MAX_RETRIES", settings.APIMaxRetries, func(v int) bool { return v >= 0 })
	if err != nil {
		return Settings{}, err
	}

	return settings, nil
}

func parseOptionalInt(env string, fallback int, valid func(int) bool) (int, error) {
	raw := strings.TrimSpace(os.Getenv(env))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", env, err)
	}
	if valid != nil && !valid(v) {
		return 0, fmt.Errorf("%s has invalid value: %d", env, v)
	}
	return v, nil
}

func parseOptionalFloat64(env string, fallback float64, valid func(float64) bool) (float64, error) {
	raw := strings.TrimSpace(os.Getenv(env))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number: %w", env, err)
	}
	if valid != nil && !valid(v) {
		return 0, fmt.Errorf("%s has invalid value: %v", env, v)
	}
	return v, nil
}

func parseOptionalDuration(env string, fallback time.Duration, valid func(time.Duration) bool) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(env))
	if raw == "" {
		return fallback, nil
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration (e.g. 45s, 2m): %w", env, err)
	}
	if valid != nil && !valid(v) {
		return 0, fmt.Errorf("%s has invalid value: %s", env, v)
	}
	return v, nil
}
