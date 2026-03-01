package web

import (
	"testing"
	"time"

	"debate/internal/orchestrator"
)

func TestDebateRunSnapshotAdjustsCursorAfterBufferTrim(t *testing.T) {
	run := newDebateRun("run-1", streamStartEvent{Problem: "p"}, func() {}, 2)
	run.appendTurn(orchestrator.Turn{Index: 1, Timestamp: time.Now().UTC()})
	run.appendTurn(orchestrator.Turn{Index: 2, Timestamp: time.Now().UTC()})
	run.appendTurn(orchestrator.Turn{Index: 3, Timestamp: time.Now().UTC()})

	turns, adjustedCursor, done, stopped, _, err := run.snapshot(0)
	if done || stopped || err != nil {
		t.Fatalf("unexpected run state done=%v stopped=%v err=%v", done, stopped, err)
	}
	if adjustedCursor != 1 {
		t.Fatalf("expected adjusted cursor=1 after one trimmed turn, got %d", adjustedCursor)
	}
	if len(turns) != 2 {
		t.Fatalf("expected 2 buffered turns, got %d", len(turns))
	}
	if turns[0].Index != 2 || turns[1].Index != 3 {
		t.Fatalf("unexpected buffered turn indexes: %#v", turns)
	}

	turns, adjustedCursor, _, _, _, _ = run.snapshot(2)
	if adjustedCursor != 2 {
		t.Fatalf("expected adjusted cursor=2, got %d", adjustedCursor)
	}
	if len(turns) != 1 || turns[0].Index != 3 {
		t.Fatalf("expected only latest turn at cursor 2, got %#v", turns)
	}
}
