package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func fallbackSummary(turns []Turn) string {
	if len(turns) == 0 {
		return "No discussion turns were generated."
	}
	last := turns[len(turns)-1]
	return fmt.Sprintf("Discussion ended without explicit consensus. Last statement by %s: %s", last.SpeakerName, last.Content)
}

func finalizeResult(res *Result, started time.Time, status string) {
	res.Status = status
	res.EndedAt = time.Now().UTC()
	res.Metrics.LatencyMS = time.Since(started).Milliseconds()
}

func ensureConsensusSummary(res *Result) {
	if strings.TrimSpace(res.Consensus.Summary) == "" {
		res.Consensus.Summary = fallbackSummary(res.Turns)
	}
}

func (o *Orchestrator) finalizeWithModerator(ctx context.Context, res *Result, started time.Time, status string, onTurn func(Turn)) (Result, error) {
	ensureConsensusSummary(res)
	finalCtx, cancel := o.callContext(ctx, started)
	finalTurn := o.appendFinalModeratorTurn(finalCtx, res, status)
	cancel()
	if reachedTokenLimit(res.Metrics.TotalTokens, o.cfg.MaxTotalTokens) {
		status = StatusTokenLimitReached
	}
	if finalTurn != nil && onTurn != nil {
		onTurn(*finalTurn)
	}
	finalizeResult(res, started, status)
	return *res, nil
}

func (o *Orchestrator) appendFinalModeratorTurn(ctx context.Context, res *Result, status string) *Turn {
	if len(res.Personas) == 0 {
		return nil
	}

	input := GenerateFinalModeratorInput{
		Problem:     res.Problem,
		Personas:    res.Personas,
		Turns:       res.Turns,
		Consensus:   res.Consensus,
		FinalStatus: status,
	}

	content := ""
	// Respect hard stop reasons without making an additional LLM call.
	if status != StatusTokenLimitReached && status != StatusDurationReached &&
		!reachedTokenLimit(res.Metrics.TotalTokens, o.cfg.MaxTotalTokens) {
		out, err := o.llm.GenerateFinalModerator(ctx, input)
		if err == nil {
			addUsage(&res.Metrics, out.Usage)
			content = strings.TrimSpace(out.Content)
		}
	}
	if content == "" {
		content = fallbackFinalModeratorContent(*res, status)
	}

	finalTurn := Turn{
		Index:       nextTurnIndex(res.Turns),
		SpeakerID:   ModeratorSpeakerID,
		SpeakerName: ModeratorSpeakerName,
		Type:        TurnTypeModerator,
		Content:     content,
		Timestamp:   time.Now().UTC(),
	}
	res.Turns = append(res.Turns, finalTurn)
	return &finalTurn
}

func nextTurnIndex(turns []Turn) int {
	if len(turns) == 0 {
		return 1
	}
	last := turns[len(turns)-1].Index
	if last > 0 {
		return last + 1
	}

	// Fallback for malformed historical data with non-positive tail indices.
	maxIdx := 0
	for _, t := range turns {
		if t.Index > maxIdx {
			maxIdx = t.Index
		}
	}
	return maxIdx + 1
}

func fallbackFinalModeratorContent(res Result, status string) string {
	summary := strings.TrimSpace(res.Consensus.Summary)
	if summary == "" {
		summary = fallbackSummary(res.Turns)
	}
	return fmt.Sprintf(
		"Final recap: %s\nOverall assessment: status=%s, consensus_score=%.2f.",
		summary,
		status,
		res.Consensus.Score,
	)
}
