package models

import (
	"sync"

	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

type ExecutionImpl struct {
	events  chan Event
	done    chan struct{}
	outcome domainRuntime.ExecutionOutcome
	mu      sync.RWMutex
}

func NewExecution() *ExecutionImpl {
	return &ExecutionImpl{
		events: make(chan Event, 16),
		done:   make(chan struct{}),
	}
}

func (e *ExecutionImpl) Events() <-chan Event {
	return e.events
}

func (e *ExecutionImpl) Done() <-chan struct{} {
	return e.done
}

func (e *ExecutionImpl) Outcome() domainRuntime.ExecutionOutcome {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.outcome
}

func (e *ExecutionImpl) Emit(
	ev Event,
) {
	select {
	case e.events <- ev:
	default:
	}
}

// Finish sets the outcome, emits EventKindEnded (best-effort), then closes
// the Events and Done channels. Outcome() is always valid after Finish returns
// regardless of whether EventKindEnded was delivered — use it as the authoritative
// completion signal if the events channel may have been full.
func (e *ExecutionImpl) Finish(
	outcome domainRuntime.ExecutionOutcome,
) {
	e.mu.Lock()
	e.outcome = outcome
	e.mu.Unlock()
	select {
	case e.events <- Event{Kind: EventKindEnded, Outcome: outcome}:
	default:
	}
	close(e.events)
	close(e.done)
}
