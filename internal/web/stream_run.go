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

	mu      sync.RWMutex
	turns   []orchestrator.Turn
	done    bool
	stopped bool
	resp    debateResponse
	runErr  error

	updates chan struct{}
}

func newDebateRun(id string, start streamStartEvent, cancel context.CancelFunc) *debateRun {
	return &debateRun{
		id:      id,
		start:   start,
		cancel:  cancel,
		updates: make(chan struct{}, 1),
	}
}

func (r *debateRun) appendTurn(turn orchestrator.Turn) {
	r.mu.Lock()
	if r.done {
		r.mu.Unlock()
		return
	}
	r.turns = append(r.turns, turn)
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

func (r *debateRun) snapshot(cursor int) ([]orchestrator.Turn, bool, bool, debateResponse, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(r.turns) {
		cursor = len(r.turns)
	}

	turns := append([]orchestrator.Turn(nil), r.turns[cursor:]...)
	return turns, r.done, r.stopped, r.resp, r.runErr
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
