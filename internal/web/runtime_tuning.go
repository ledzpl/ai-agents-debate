package web

import (
	"context"
	"fmt"
	"strings"
	"time"

	"debate/internal/orchestrator"
)

const maxDurationSeconds = int64(1<<63-1) / int64(time.Second)

func (r *debateRequest) validateRuntimeTuning() error {
	if r == nil {
		return nil
	}
	if r.AudienceMode != nil {
		mode := strings.ToLower(strings.TrimSpace(*r.AudienceMode))
		switch mode {
		case orchestrator.AudienceModeGeneral, orchestrator.AudienceModeExpert:
			*r.AudienceMode = mode
		default:
			return fmt.Errorf("audience_mode must be one of: %s, %s", orchestrator.AudienceModeGeneral, orchestrator.AudienceModeExpert)
		}
	}
	if err := validateMinInt("max_turns", r.MaxTurns, 0); err != nil {
		return err
	}
	if err := validateRangeFloat("consensus_threshold", r.ConsensusThreshold, 0, 1); err != nil {
		return err
	}
	if err := validateMinInt("max_no_progress_judges", r.MaxNoProgressJudges, 1); err != nil {
		return err
	}
	if err := validateMinFloat("no_progress_epsilon", r.NoProgressEpsilon, 0); err != nil {
		return err
	}
	if err := validateMinInt("unlimited_hard_max_turns", r.UnlimitedHardMaxTurns, 1); err != nil {
		return err
	}
	if err := validateMinInt("direct_handoff_judge_every", r.DirectHandoffJudgeEvery, 1); err != nil {
		return err
	}
	if err := validateMinInt("llm_history_turn_window", r.LLMHistoryTurnWindow, 1); err != nil {
		return err
	}
	if err := validateMinInt("max_total_tokens", r.MaxTotalTokens, 1); err != nil {
		return err
	}
	if err := validateDurationSeconds("max_duration_seconds", r.MaxDurationSeconds); err != nil {
		return err
	}
	if err := validateDurationSeconds("run_timeout_seconds", r.RunTimeoutSeconds); err != nil {
		return err
	}
	return nil
}

func (r debateRequest) hasRunnerTuning() bool {
	return r.AudienceMode != nil ||
		r.MaxTurns != nil ||
		r.ConsensusThreshold != nil ||
		r.MaxNoProgressJudges != nil ||
		r.NoProgressEpsilon != nil ||
		r.UnlimitedHardMaxTurns != nil ||
		r.DirectHandoffJudgeEvery != nil ||
		r.LLMHistoryTurnWindow != nil ||
		r.MaxDurationSeconds != nil ||
		r.MaxTotalTokens != nil
}

func (r debateRequest) hasRunTimeoutOverride() bool {
	return r.RunTimeoutSeconds != nil
}

func (r debateRequest) runTimeoutDuration() time.Duration {
	if r.RunTimeoutSeconds == nil {
		return 0
	}
	return time.Duration(*r.RunTimeoutSeconds) * time.Second
}

func (r debateRequest) applyRunnerTuning(base orchestrator.Config) orchestrator.Config {
	cfg := base
	if r.AudienceMode != nil {
		cfg.AudienceMode = strings.ToLower(strings.TrimSpace(*r.AudienceMode))
	}
	if r.MaxTurns != nil {
		cfg.MaxTurns = *r.MaxTurns
	}
	if r.ConsensusThreshold != nil {
		cfg.ConsensusThreshold = *r.ConsensusThreshold
	}
	if r.MaxNoProgressJudges != nil {
		cfg.MaxNoProgressJudges = *r.MaxNoProgressJudges
	}
	if r.NoProgressEpsilon != nil {
		cfg.NoProgressEpsilon = *r.NoProgressEpsilon
	}
	if r.UnlimitedHardMaxTurns != nil {
		cfg.UnlimitedHardMaxTurns = *r.UnlimitedHardMaxTurns
	}
	if r.DirectHandoffJudgeEvery != nil {
		cfg.DirectHandoffJudgeEvery = *r.DirectHandoffJudgeEvery
	}
	if r.LLMHistoryTurnWindow != nil {
		cfg.LLMHistoryTurnWindow = *r.LLMHistoryTurnWindow
	}
	if r.MaxDurationSeconds != nil {
		cfg.MaxDuration = time.Duration(*r.MaxDurationSeconds) * time.Second
	}
	if r.MaxTotalTokens != nil {
		cfg.MaxTotalTokens = *r.MaxTotalTokens
	}
	return cfg
}

func (a *App) contextWithRuntimeTimeout(parent context.Context, req debateRequest) (context.Context, context.CancelFunc, error) {
	if !req.hasRunTimeoutOverride() {
		return parent, nil, nil
	}
	if req.RunTimeoutSeconds == nil {
		return parent, nil, nil
	}
	if int64(*req.RunTimeoutSeconds) > maxDurationSeconds {
		return nil, nil, fmt.Errorf("run_timeout_seconds is too large")
	}
	duration := req.runTimeoutDuration()
	ctx, cancel := context.WithTimeout(parent, duration)
	return ctx, cancel, nil
}

func validateMinInt(name string, value *int, min int) error {
	if value == nil {
		return nil
	}
	if *value < min {
		return fmt.Errorf("%s must be >= %d", name, min)
	}
	return nil
}

func validateRangeFloat(name string, value *float64, min float64, max float64) error {
	if value == nil {
		return nil
	}
	if *value < min || *value > max {
		return fmt.Errorf("%s must be between %.2f and %.2f", name, min, max)
	}
	return nil
}

func validateMinFloat(name string, value *float64, minExclusive float64) error {
	if value == nil {
		return nil
	}
	if *value <= minExclusive {
		return fmt.Errorf("%s must be > %.0f", name, minExclusive)
	}
	return nil
}

func validateDurationSeconds(name string, value *int) error {
	if value == nil {
		return nil
	}
	if *value <= 0 {
		return fmt.Errorf("%s must be >= 1", name)
	}
	if int64(*value) > maxDurationSeconds {
		return fmt.Errorf("%s is too large", name)
	}
	return nil
}
