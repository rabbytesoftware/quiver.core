package mocks

import (
	"context"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/domain"
	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard"
)

type Wizard struct {
	mu            sync.Mutex
	ExecuteErr    error
	ExecuteCalled bool
	CancelledNS   domain.Namespace
	// BlockExecute, when non-nil, causes Execute to block until the channel is closed.
	BlockExecute chan struct{}
}

func (m *Wizard) Execute(ctx context.Context, _ wizard.RunRequest, _ wizard.StepReporter) error {
	m.mu.Lock()
	m.ExecuteCalled = true
	block := m.BlockExecute
	m.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return m.ExecuteErr
}

func (m *Wizard) Cancel(ns domain.Namespace) {
	m.mu.Lock()
	m.CancelledNS = ns
	m.mu.Unlock()
}

func (m *Wizard) Shutdown(_ context.Context) error {
	return nil
}

func (m *Wizard) RegisterDispatch(_ domainstep.StepType, _ wizard.DispatchFn) {}

func (m *Wizard) WasExecuteCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ExecuteCalled
}

func (m *Wizard) WasCancelledFor(ns domain.Namespace) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.CancelledNS == ns
}
