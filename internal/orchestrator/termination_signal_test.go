package orchestrator

import "testing"

func TestParseTurnTerminationSignal(t *testing.T) {
	signal := parseTurnTerminationSignal("핵심만 정리합니다.\nNEXT: b\nCLOSE: yes\nNEW_POINT: no")
	if signal.closeVote == nil || !*signal.closeVote {
		t.Fatalf("expected closeVote=true, got %#v", signal.closeVote)
	}
	if signal.newPoint == nil || *signal.newPoint {
		t.Fatalf("expected newPoint=false, got %#v", signal.newPoint)
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
		{Type: TurnTypePersona, SpeakerID: "b", SpeakerName: "B", Content: "NEXT: c\nCLOSE: yes\nNEW_POINT: no"},
		{Type: TurnTypePersona, SpeakerID: "c", SpeakerName: "C", Content: "NEXT: a\nCLOSE: yes\nNEW_POINT: no"},
	}
	for _, turn := range turns {
		tracker.observe(turn)
	}
	if !tracker.shouldSuggestStop(3) {
		t.Fatal("expected close quorum + no-new-point streak to suggest stop")
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
