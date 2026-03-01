package orchestrator

import (
	"context"
	"errors"
	"time"
)

func (o *Orchestrator) callContext(ctx context.Context, started time.Time) (context.Context, context.CancelFunc) {
	if o.cfg.MaxDuration <= 0 {
		return ctx, func() {}
	}
	deadline := started.Add(o.cfg.MaxDuration)
	if parentDeadline, ok := ctx.Deadline(); ok && parentDeadline.Before(deadline) {
		deadline = parentDeadline
	}
	return context.WithDeadline(ctx, deadline)
}

func (o *Orchestrator) durationStatusOnLLMError(started time.Time, err error) (string, bool) {
	if err == nil {
		return "", false
	}
	if errors.Is(err, context.DeadlineExceeded) && reachedDurationLimit(started, o.cfg.MaxDuration) {
		return StatusDurationReached, true
	}
	return "", false
}

func shouldJudgeConsensus(turnIndex int, personaCount int) bool {
	if personaCount <= 0 {
		return true
	}
	return (turnIndex+1)%personaCount == 0
}

func hasNextPersonaTurn(turnIndex int, maxTurns int) bool {
	if maxTurns <= 0 {
		return true
	}
	return turnIndex+1 < maxTurns
}

func reachedDurationLimit(started time.Time, maxDuration time.Duration) bool {
	return maxDuration > 0 && time.Since(started) >= maxDuration
}

func reachedTokenLimit(totalTokens int, maxTotalTokens int) bool {
	return maxTotalTokens > 0 && totalTokens >= maxTotalTokens
}
