package mocks

import (
	"context"

	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

// Wizard is a test double for wizard.Wizard.
type Wizard struct {
	StartFn        func(ctx context.Context, req wizard.RunRequest) wizard.Execution
	ShutdownFn     func(ctx context.Context) error
	ProcessAliveFn func(pid int) bool
	// BlockStart, when non-nil, blocks Start until the channel is closed.
	BlockStart chan struct{}
}

func (m *Wizard) Start(
	ctx context.Context,
	req wizard.RunRequest,
) wizard.Execution {
	if m.BlockStart != nil {
		<-m.BlockStart
	}
	if m.StartFn != nil {
		return m.StartFn(ctx, req)
	}
	return NewDoneExecution(domainRuntime.ExecutionOutcomeSuccess)
}

func (m *Wizard) Shutdown(ctx context.Context) error {
	if m.ShutdownFn != nil {
		return m.ShutdownFn(ctx)
	}
	return nil
}

func (m *Wizard) ProcessAlive(pid int) bool {
	if m.ProcessAliveFn != nil {
		return m.ProcessAliveFn(pid)
	}
	return false
}

// doneExecution is an already-finished Execution used by the mock.
type doneExecution struct {
	events  chan wizard.Event
	done    chan struct{}
	outcome domainRuntime.ExecutionOutcome
}

func NewDoneExecution(
	outcome domainRuntime.ExecutionOutcome,
) wizard.Execution {
	e := &doneExecution{
		events:  make(chan wizard.Event, 1),
		done:    make(chan struct{}),
		outcome: outcome,
	}
	e.events <- wizard.Event{Kind: wizard.EventKindEnded, Outcome: outcome}
	close(e.events)
	close(e.done)
	return e
}

func (e *doneExecution) Events() <-chan wizard.Event             { return e.events }
func (e *doneExecution) Done() <-chan struct{}                   { return e.done }
func (e *doneExecution) Outcome() domainRuntime.ExecutionOutcome { return e.outcome }
