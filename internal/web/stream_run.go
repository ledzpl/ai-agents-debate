package web

import (
	"context"
	"errors"
	"sync"
	"time"

	"debate/internal/orchestrator"
)

type debateRun struct {
	id    string
	start streamStartEvent

	cancel context.CancelFunc

	mu         sync.RWMutex
	turns      []orchestrator.Turn
	baseCursor int
	maxTurns   int
	done       bool
	stopped    bool
	resp       debateResponse
	runErr     error

	updates chan struct{}
}

func newDebateRun(id string, start streamStartEvent, cancel context.CancelFunc, maxTurns int) *debateRun {
	return &debateRun{
		id:       id,
		start:    start,
		cancel:   cancel,
		maxTurns: maxTurns,
		updates:  make(chan struct{}, 1),
	}
}

func (r *debateRun) appendTurn(turn orchestrator.Turn) {
	r.mu.Lock()
	if r.done {
		r.mu.Unlock()
		return
	}
	r.turns = append(r.turns, turn)
	if r.maxTurns > 0 && len(r.turns) > r.maxTurns {
		drop := len(r.turns) - r.maxTurns
		r.turns = append([]orchestrator.Turn(nil), r.turns[drop:]...)
		r.baseCursor += drop
	}
	r.mu.Unlock()
	r.notify()
}

func (r *debateRun) stop() {
	r.mu.Lock()
	if r.done {
		r.mu.Unlock()
		return
	}
	r.stopped = true
	r.mu.Unlock()
	r.cancel()
	r.notify()
}

func (r *debateRun) finish(resp debateResponse, runErr error) {
	r.mu.Lock()
	if r.done {
		r.mu.Unlock()
		return
	}

	if runErr != nil && r.stopped && errors.Is(runErr, context.Canceled) {
		runErr = nil
	}

	r.resp = resp
	r.runErr = runErr
	r.done = true
	r.mu.Unlock()
	r.notify()
}

func (r *debateRun) snapshot(cursor int) ([]orchestrator.Turn, int, bool, bool, debateResponse, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if cursor < r.baseCursor {
		cursor = r.baseCursor
	}
	if cursor < 0 {
		cursor = 0
	}
	localCursor := cursor - r.baseCursor
	if localCursor < 0 {
		localCursor = 0
	}
	if localCursor > len(r.turns) {
		localCursor = len(r.turns)
	}

	turns := append([]orchestrator.Turn(nil), r.turns[localCursor:]...)
	return turns, cursor, r.done, r.stopped, r.resp, r.runErr
}

func (r *debateRun) waitForUpdate(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.updates:
		return nil
	}
}

func (r *debateRun) notify() {
	select {
	case r.updates <- struct{}{}:
	default:
	}
}

const runRetention = 10 * time.Minute
