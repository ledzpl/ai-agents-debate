package orchestrator

import (
	"strings"
	"testing"
)

func TestParseTurnTerminationSignal(t *testing.T) {
	signal := parseTurnTerminationSignal("핵심만 정리합니다.\nNEXT: b\nCLOSE: yes\nNEW_POINT: no")
	if signal.closeVote == nil || !*signal.closeVote {
		t.Fatalf("expected closeVote=true, got %#v", signal.closeVote)
	}
	if signal.newPoint == nil || *signal.newPoint {
		t.Fatalf("expected newPoint=false, got %#v", signal.newPoint)
	}
	if signal.persuasionAdopted {
		t.Fatalf("did not expect persuasion adoption signal in basic close/new_point content")
	}
	if signal.boundedExperiment {
		t.Fatalf("did not expect bounded experiment signal in basic close/new_point content")
	}
}

func TestParseTurnTerminationSignalRecognizesPersuasionAndExperimentSignals(t *testing.T) {
	content := strings.Join([]string{
		"1. PERSUASION_UPDATE: changed=yes; adopted=리스크 가드레일; rationale=장애 비용 근거; remaining_gap=none",
		"- DECISION_CHECK: choose Option A or B; owner=pm; decide_by=2026-03-15; success_metric=p95<300ms; stop_condition=error_rate>1%",
		"CLOSE: yes",
		"NEW_POINT: no",
	}, "\n")
	signal := parseTurnTerminationSignal(content)
	if !signal.persuasionAdopted {
		t.Fatalf("expected persuasion adoption signal from PERSUASION_UPDATE line")
	}
	if !signal.boundedExperiment {
		t.Fatalf("expected bounded experiment signal from decision-check line")
	}
}

func TestRequiredCloseVotes(t *testing.T) {
	tests := []struct {
		personaCount int
		want         int
	}{
		{personaCount: 1, want: 1},
		{personaCount: 2, want: 2},
		{personaCount: 3, want: 2},
		{personaCount: 4, want: 3},
		{personaCount: 5, want: 4},
	}
	for _, tc := range tests {
		got := requiredCloseVotes(tc.personaCount)
		if got != tc.want {
			t.Fatalf("personaCount=%d got=%d want=%d", tc.personaCount, got, tc.want)
		}
	}
}

func TestTerminationSignalTrackerShouldSuggestStop(t *testing.T) {
	tracker := newTerminationSignalTracker()
	turns := []Turn{
		{Type: TurnTypePersona, SpeakerID: "a", SpeakerName: "A", Content: "NEXT: b\nCLOSE: yes\nNEW_POINT: no"},
		{Type: TurnTypePersona, SpeakerID: "b", SpeakerName: "B", Content: "NEXT: c\nPERSUASION_UPDATE: changed=yes; adopted=A의 가드레일; rationale=리스크 근거; remaining_gap=none\nCLOSE: yes\nNEW_POINT: no"},
		{Type: TurnTypePersona, SpeakerID: "c", SpeakerName: "C", Content: "NEXT: a\nCLOSE: yes\nNEW_POINT: no"},
	}
	for _, turn := range turns {
		tracker.observe(turn)
	}
	if !tracker.shouldSuggestStop(3) {
		t.Fatal("expected close quorum + no-new-point streak + persuasion/experiment signal to suggest stop")
	}
}

func TestTerminationSignalTrackerDoesNotSuggestStopWithoutPersuasionOrExperimentSignal(t *testing.T) {
	tracker := newTerminationSignalTracker()
	turns := []Turn{
		{Type: TurnTypePersona, SpeakerID: "a", SpeakerName: "A", Content: "NEXT: b\nCLOSE: yes\nNEW_POINT: no"},
		{Type: TurnTypePersona, SpeakerID: "b", SpeakerName: "B", Content: "NEXT: c\nCLOSE: yes\nNEW_POINT: no"},
		{Type: TurnTypePersona, SpeakerID: "c", SpeakerName: "C", Content: "NEXT: a\nCLOSE: yes\nNEW_POINT: no"},
	}
	for _, turn := range turns {
		tracker.observe(turn)
	}
	if tracker.shouldSuggestStop(3) {
		t.Fatal("did not expect stop suggestion without persuasion/experiment signal")
	}
}

func TestTerminationSignalTrackerKeepsStreakWhenSignalMissing(t *testing.T) {
	tracker := newTerminationSignalTracker()
	tracker.observe(Turn{Type: TurnTypePersona, SpeakerID: "a", Content: "CLOSE: yes\nNEW_POINT: no"})
	if tracker.noNewPointStreak != 1 {
		t.Fatalf("expected streak=1, got %d", tracker.noNewPointStreak)
	}
	tracker.observe(Turn{Type: TurnTypePersona, SpeakerID: "b", Content: "NEXT: c"})
	if tracker.noNewPointStreak != 1 {
		t.Fatalf("missing NEW_POINT should keep streak, got %d", tracker.noNewPointStreak)
	}
}
