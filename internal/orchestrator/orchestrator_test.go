package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"debate/internal/persona"
)

type fakeLLM struct {
	generateCalls    int
	moderatorCalls   int
	finalCalls       int
	selectCalls      int
	judgeAtTurn      int
	openingSpeakerID string
	selectDelay      time.Duration
	turnDelay        time.Duration
	judgeDelay       time.Duration
	moderatorDelay   time.Duration
	// Optional override for judge summary. Empty string is allowed.
	useCustomJudgeSummary bool
	judgeSummary          string
}

func (f *fakeLLM) GenerateTurn(ctx context.Context, input GenerateTurnInput) (GenerateTurnOutput, error) {
	if err := waitWithContext(ctx, f.turnDelay); err != nil {
		return GenerateTurnOutput{}, err
	}
	f.generateCalls++
	return GenerateTurnOutput{
		Content: fmt.Sprintf("turn %d by %s", f.generateCalls, input.Speaker.Name),
		Usage: Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}, nil
}

func (f *fakeLLM) GenerateModerator(ctx context.Context, input GenerateModeratorInput) (GenerateModeratorOutput, error) {
	if err := waitWithContext(ctx, f.moderatorDelay); err != nil {
		return GenerateModeratorOutput{}, err
	}
	f.moderatorCalls++
	return GenerateModeratorOutput{
		Content: "moderator summary before " + input.NextSpeaker.Name,
		Usage: Usage{
			PromptTokens:     3,
			CompletionTokens: 3,
			TotalTokens:      6,
		},
	}, nil
}

func (f *fakeLLM) GenerateFinalModerator(_ context.Context, _ GenerateFinalModeratorInput) (GenerateFinalModeratorOutput, error) {
	f.finalCalls++
	return GenerateFinalModeratorOutput{
		Content: "final moderator wrap-up",
		Usage: Usage{
			PromptTokens:     4,
			CompletionTokens: 4,
			TotalTokens:      8,
		},
	}, nil
}

func (f *fakeLLM) SelectOpeningSpeaker(ctx context.Context, input SelectOpeningSpeakerInput) (SelectOpeningSpeakerOutput, error) {
	if err := waitWithContext(ctx, f.selectDelay); err != nil {
		return SelectOpeningSpeakerOutput{}, err
	}
	f.selectCalls++

	selectedID := strings.TrimSpace(f.openingSpeakerID)
	if selectedID == "" && len(input.Personas) > 0 {
		selectedID = input.Personas[0].ID
	}
	return SelectOpeningSpeakerOutput{
		PersonaID: selectedID,
		Usage: Usage{
			PromptTokens:     1,
			CompletionTokens: 1,
			TotalTokens:      2,
		},
	}, nil
}

func (f *fakeLLM) JudgeConsensus(ctx context.Context, input JudgeConsensusInput) (JudgeConsensusOutput, error) {
	if err := waitWithContext(ctx, f.judgeDelay); err != nil {
		return JudgeConsensusOutput{}, err
	}
	reached := len(input.Turns) >= f.judgeAtTurn
	score := 0.2
	if reached {
		score = 0.9
	}
	summary := "summary"
	if f.useCustomJudgeSummary {
		summary = f.judgeSummary
	}
	return JudgeConsensusOutput{
		Consensus: Consensus{
			Reached: reached,
			Score:   score,
			Summary: summary,
		},
		Usage: Usage{
			PromptTokens:     2,
			CompletionTokens: 2,
			TotalTokens:      4,
		},
	}, nil
}

func testPersonas() []persona.Persona {
	return []persona.Persona{
		{ID: "a", Name: "Architect", Role: "architecture"},
		{ID: "o", Name: "Operator", Role: "operations"},
	}
}

func waitWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func TestRunStopsOnConsensus(t *testing.T) {
	llm := &fakeLLM{judgeAtTurn: 3}
	orch := New(llm, Config{MaxTurns: 8, ConsensusThreshold: 0.75})
	result, err := orch.Run(context.Background(), "How do we reduce incidents?", testPersonas(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if result.Status != StatusConsensusReached {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if len(result.Turns) != 8 {
		t.Fatalf("expected 8 turns with stricter consensus confirmation, got %d", len(result.Turns))
	}
	if llm.moderatorCalls != 3 {
		t.Fatalf("expected 3 moderator calls, got %d", llm.moderatorCalls)
	}
	if llm.finalCalls != 1 {
		t.Fatalf("expected 1 final moderator call, got %d", llm.finalCalls)
	}
	if result.Turns[len(result.Turns)-1].Type != TurnTypeModerator {
		t.Fatalf("expected final turn to be moderator, got %s", result.Turns[len(result.Turns)-1].Type)
	}
}

func TestRunWithNilLLMReturnsError(t *testing.T) {
	orch := New(nil, Config{})
	result, err := orch.Run(context.Background(), "topic", testPersonas(), nil)
	if err == nil {
		t.Fatal("expected error for nil llm client")
	}
	if !strings.Contains(err.Error(), "llm client is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusError {
		t.Fatalf("expected status %q, got %q", StatusError, result.Status)
	}
}

func TestRunStopsOnMaxTurns(t *testing.T) {
	llm := &fakeLLM{judgeAtTurn: 99}
	orch := New(llm, Config{MaxTurns: 4, ConsensusThreshold: 0.75})
	result, err := orch.Run(context.Background(), "How do we reduce incidents?", testPersonas(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if result.Status != StatusMaxTurnsReached {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if len(result.Turns) != 8 {
		t.Fatalf("expected 8 turns, got %d", len(result.Turns))
	}
	if llm.moderatorCalls != 3 {
		t.Fatalf("expected 3 moderator calls, got %d", llm.moderatorCalls)
	}
	if llm.finalCalls != 1 {
		t.Fatalf("expected 1 final moderator call, got %d", llm.finalCalls)
	}
	if result.Turns[len(result.Turns)-1].Type != TurnTypeModerator {
		t.Fatalf("expected final turn to be moderator, got %s", result.Turns[len(result.Turns)-1].Type)
	}
}

func TestShouldJudgeConsensus(t *testing.T) {
	tests := []struct {
		turnIndex    int
		personaCount int
		want         bool
	}{
		{turnIndex: 0, personaCount: 2, want: false},
		{turnIndex: 1, personaCount: 2, want: true},
		{turnIndex: 2, personaCount: 2, want: false},
		{turnIndex: 7, personaCount: 2, want: true},
		{turnIndex: 0, personaCount: 0, want: true},
	}

	for _, tc := range tests {
		got := shouldJudgeConsensus(tc.turnIndex, tc.personaCount)
		if got != tc.want {
			t.Fatalf("turnIndex=%d personaCount=%d got=%v want=%v", tc.turnIndex, tc.personaCount, got, tc.want)
		}
	}
}

func TestHasNextPersonaTurn(t *testing.T) {
	if !hasNextPersonaTurn(0, 0) {
		t.Fatal("expected unbounded mode to always have next turn")
	}
	if !hasNextPersonaTurn(0, 2) {
		t.Fatal("expected next turn at index 0")
	}
	if hasNextPersonaTurn(1, 2) {
		t.Fatal("did not expect next turn at final index")
	}
}

func TestNewInvalidConsensusThresholdUsesDefault(t *testing.T) {
	orch := New(&fakeLLM{judgeAtTurn: 3}, Config{ConsensusThreshold: 1.5})
	if orch.cfg.ConsensusThreshold != defaultConsensusThreshold {
		t.Fatalf("expected default threshold %.2f, got %.2f", defaultConsensusThreshold, orch.cfg.ConsensusThreshold)
	}
}

func TestRunUnlimitedStopsOnConsensus(t *testing.T) {
	llm := &fakeLLM{judgeAtTurn: 7}
	orch := New(llm, Config{
		MaxTurns:            0,
		ConsensusThreshold:  0.75,
		MaxNoProgressJudges: 10,
	})
	result, err := orch.Run(context.Background(), "How do we reduce incidents?", testPersonas(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if result.Status != StatusConsensusReached {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if llm.generateCalls != 6 {
		t.Fatalf("expected 6 persona turns, got %d", llm.generateCalls)
	}
	if llm.moderatorCalls != 5 {
		t.Fatalf("expected 5 moderator turns, got %d", llm.moderatorCalls)
	}
	if llm.finalCalls != 1 {
		t.Fatalf("expected 1 final moderator turn, got %d", llm.finalCalls)
	}
	if result.Turns[len(result.Turns)-1].Type != TurnTypeModerator {
		t.Fatalf("expected final turn to be moderator, got %s", result.Turns[len(result.Turns)-1].Type)
	}
}

func TestRunUnlimitedStopsOnNoProgress(t *testing.T) {
	llm := &fakeLLM{judgeAtTurn: 999}
	orch := New(llm, Config{
		MaxTurns:            0,
		ConsensusThreshold:  0.75,
		MaxNoProgressJudges: 2,
		NoProgressEpsilon:   0.0001,
	})
	result, err := orch.Run(context.Background(), "How do we reduce incidents?", testPersonas(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if result.Status != StatusNoProgressReached {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if llm.generateCalls != 6 {
		t.Fatalf("expected 6 persona turns, got %d", llm.generateCalls)
	}
	if llm.finalCalls != 1 {
		t.Fatalf("expected 1 final moderator turn, got %d", llm.finalCalls)
	}
	if result.Turns[len(result.Turns)-1].Type != TurnTypeModerator {
		t.Fatalf("expected final turn to be moderator, got %s", result.Turns[len(result.Turns)-1].Type)
	}
}

func TestRunStopsOnTokenLimitWithoutFinalModeratorLLMCall(t *testing.T) {
	llm := &fakeLLM{judgeAtTurn: 999}
	orch := New(llm, Config{
		MaxTurns:          0,
		MaxTotalTokens:    10, // one persona turn already exceeds this (15)
		MaxDuration:       time.Hour,
		NoProgressEpsilon: 0.0001,
	})
	result, err := orch.Run(context.Background(), "How do we reduce incidents?", testPersonas(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if result.Status != StatusTokenLimitReached {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if llm.finalCalls != 0 {
		t.Fatalf("expected no final moderator LLM call, got %d", llm.finalCalls)
	}
	if llm.moderatorCalls != 0 {
		t.Fatalf("expected no moderator calls before token stop, got %d", llm.moderatorCalls)
	}
	if len(result.Turns) != 2 {
		t.Fatalf("expected persona + final moderator turns, got %d", len(result.Turns))
	}
	if result.Turns[len(result.Turns)-1].Type != TurnTypeModerator {
		t.Fatalf("expected final turn to be moderator, got %s", result.Turns[len(result.Turns)-1].Type)
	}
}

func TestRunStopsOnDurationWhenLLMCallExceedsDeadline(t *testing.T) {
	llm := &fakeLLM{
		judgeAtTurn: 999,
		turnDelay:   40 * time.Millisecond,
	}
	orch := New(llm, Config{
		MaxTurns:            0,
		MaxDuration:         10 * time.Millisecond,
		MaxTotalTokens:      100000,
		MaxNoProgressJudges: 10,
	})

	result, err := orch.Run(context.Background(), "How do we reduce incidents?", testPersonas(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if result.Status != StatusDurationReached {
		t.Fatalf("expected status=%s, got %s", StatusDurationReached, result.Status)
	}
	if llm.generateCalls != 0 {
		t.Fatalf("expected no completed turn due deadline, got %d", llm.generateCalls)
	}
	if llm.finalCalls != 0 {
		t.Fatalf("expected no final moderator llm call on duration stop, got %d", llm.finalCalls)
	}
	if len(result.Turns) != 1 {
		t.Fatalf("expected fallback final moderator only, got %d turns", len(result.Turns))
	}
	if result.Turns[0].Type != TurnTypeModerator {
		t.Fatalf("expected final fallback moderator turn, got %s", result.Turns[0].Type)
	}
}

func TestConsensusFallbackSummaryUsesPreFinalTurn(t *testing.T) {
	llm := &fakeLLM{
		judgeAtTurn:           999,
		useCustomJudgeSummary: true,
		judgeSummary:          "",
	}
	orch := New(llm, Config{
		MaxTurns:           1,
		ConsensusThreshold: 0.75,
	})
	result, err := orch.Run(context.Background(), "How do we reduce incidents?", testPersonas(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if result.Status != StatusMaxTurnsReached {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if strings.Contains(result.Consensus.Summary, ModeratorSpeakerName) {
		t.Fatalf("summary should not be based on final moderator turn: %q", result.Consensus.Summary)
	}
	if !strings.Contains(result.Consensus.Summary, "Architect") {
		t.Fatalf("expected summary to reference pre-final persona turn: %q", result.Consensus.Summary)
	}
}

func TestNextTurnIndex(t *testing.T) {
	if got := nextTurnIndex(nil); got != 1 {
		t.Fatalf("expected 1 for empty turns, got %d", got)
	}

	if got := nextTurnIndex([]Turn{{Index: 1}, {Index: 2}}); got != 3 {
		t.Fatalf("expected fast-path next index 3, got %d", got)
	}

	if got := nextTurnIndex([]Turn{{Index: 3}, {Index: 0}, {Index: -1}}); got != 4 {
		t.Fatalf("expected fallback max+1 index 4, got %d", got)
	}
}

func TestRequiredConsensusConfirmations(t *testing.T) {
	if got := requiredConsensusConfirmations(0); got != 1 {
		t.Fatalf("expected single confirmation for non-positive persona count, got %d", got)
	}
	if got := requiredConsensusConfirmations(1); got != 1 {
		t.Fatalf("expected single confirmation for one persona, got %d", got)
	}
	if got := requiredConsensusConfirmations(2); got != defaultConsensusConfirmations {
		t.Fatalf("expected %d confirmations for multi-persona, got %d", defaultConsensusConfirmations, got)
	}
}

func TestDefaultOpeningSpeakerIndexMatchesProblemContext(t *testing.T) {
	personas := []persona.Persona{
		{ID: "mkt", Name: "Marketing Lead", Role: "growth messaging and user acquisition"},
		{ID: "sec", Name: "Security Analyst", Role: "security incident response and threat modeling"},
	}
	got := defaultOpeningSpeakerIndex("How should we design security incident response playbooks?", personas)
	if got != 1 {
		t.Fatalf("expected security persona at index 1, got %d", got)
	}
}

func TestRunUsesSelectedOpeningSpeaker(t *testing.T) {
	llm := &fakeLLM{
		judgeAtTurn:      999,
		openingSpeakerID: "o",
	}
	orch := New(llm, Config{MaxTurns: 1, ConsensusThreshold: 0.75})

	result, err := orch.Run(context.Background(), "How do we reduce incidents?", testPersonas(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if llm.selectCalls != 1 {
		t.Fatalf("expected 1 opening speaker selection call, got %d", llm.selectCalls)
	}
	if len(result.Turns) < 1 {
		t.Fatalf("expected at least one turn, got %d", len(result.Turns))
	}
	if result.Turns[0].SpeakerID != "o" {
		t.Fatalf("expected first persona speaker to be 'o', got %q", result.Turns[0].SpeakerID)
	}
}

func TestRunIgnoresOpeningSpeakerSelectionByName(t *testing.T) {
	llm := &fakeLLM{
		judgeAtTurn:      999,
		openingSpeakerID: "Operator", // name, not persona id
	}
	orch := New(llm, Config{MaxTurns: 1, ConsensusThreshold: 0.75})

	result, err := orch.Run(context.Background(), "generic topic", testPersonas(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(result.Turns) < 1 {
		t.Fatalf("expected at least one turn, got %d", len(result.Turns))
	}
	if result.Turns[0].SpeakerID != "a" {
		t.Fatalf("expected fallback opening speaker id 'a', got %q", result.Turns[0].SpeakerID)
	}
}

func TestFinalizeStatusDowngradesToTokenLimitWhenFinalModeratorExceedsCap(t *testing.T) {
	llm := &fakeLLM{
		judgeAtTurn: 999,
	}
	orch := New(llm, Config{
		MaxTurns:           1,
		ConsensusThreshold: 0.75,
		MaxTotalTokens:     25, // 15(persona)+4(judge)+8(final)=27
	})

	result, err := orch.Run(context.Background(), "topic", testPersonas(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if result.Status != StatusTokenLimitReached {
		t.Fatalf("expected status=%s, got %s", StatusTokenLimitReached, result.Status)
	}
}
