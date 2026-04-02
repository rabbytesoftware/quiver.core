package mocks

import (
	"context"
	"os"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/models"
)

// Process is a mock for process.Process.
// Configure behavior with the error fields or override methods with Func fields.
type Process struct {
	id      string
	status  models.Status
	mu      sync.RWMutex

	StopErr  error
	KillErr  error
	CloseErr error

	SignalFunc func(sig os.Signal) error
}

func NewProcess(
	id string,
	status models.Status,
) *Process {
	return &Process{
		id:     id,
		status: status,
	}
}

func (m *Process) ID() string {
	return m.id
}

func (m *Process) Key() string {
	return m.id
}

func (m *Process) PID() int {
	return -1
}

func (m *Process) Status() models.Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *Process) Start(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = models.StatusRunning
	return nil
}

func (m *Process) Stop(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.StopErr != nil {
		return m.StopErr
	}
	m.status = models.StatusFinished
	return nil
}

func (m *Process) Kill(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.KillErr != nil {
		return m.KillErr
	}
	m.status = models.StatusFinished
	return nil
}

func (m *Process) Wait(_ context.Context) error {
	return nil
}

func (m *Process) Signal(sig os.Signal) error {
	if m.SignalFunc != nil {
		return m.SignalFunc(sig)
	}
	return nil
}

func (m *Process) Done() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (m *Process) Close() error {
	return m.CloseErr
}

func (m *Process) ExitCode() int {
	return 0
}

func (m *Process) Output() string {
	return ""
}

func (m *Process) Error() string {
	return ""
}

func (m *Process) StreamOutput() <-chan string {
	ch := make(chan string)
	close(ch)
	return ch
}

func (m *Process) StreamError() <-chan string {
	ch := make(chan string)
	close(ch)
	return ch
}
