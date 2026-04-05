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
}

func (m *Wizard) Execute(_ context.Context, _ wizard.RunRequest, _ wizard.StepReporter) error {
	m.mu.Lock()
	m.ExecuteCalled = true
	m.mu.Unlock()
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
