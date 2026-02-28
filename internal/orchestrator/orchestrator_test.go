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
	generateCalls  int
	moderatorCalls int
	finalCalls     int
	judgeAtTurn    int
	// Optional override for judge summary. Empty string is allowed.
	useCustomJudgeSummary bool
	judgeSummary          string
}

func (f *fakeLLM) GenerateTurn(_ context.Context, input GenerateTurnInput) (GenerateTurnOutput, error) {
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

func (f *fakeLLM) GenerateModerator(_ context.Context, input GenerateModeratorInput) (GenerateModeratorOutput, error) {
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

func (f *fakeLLM) JudgeConsensus(_ context.Context, input JudgeConsensusInput) (JudgeConsensusOutput, error) {
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
	if len(result.Turns) != 4 {
		t.Fatalf("expected 4 turns, got %d", len(result.Turns))
	}
	if llm.moderatorCalls != 1 {
		t.Fatalf("expected 1 moderator call, got %d", llm.moderatorCalls)
	}
	if llm.finalCalls != 1 {
		t.Fatalf("expected 1 final moderator call, got %d", llm.finalCalls)
	}
	if result.Turns[len(result.Turns)-1].Type != TurnTypeModerator {
		t.Fatalf("expected final turn to be moderator, got %s", result.Turns[len(result.Turns)-1].Type)
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
	if llm.generateCalls != 4 {
		t.Fatalf("expected 4 persona turns, got %d", llm.generateCalls)
	}
	if llm.moderatorCalls != 3 {
		t.Fatalf("expected 3 moderator turns, got %d", llm.moderatorCalls)
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
