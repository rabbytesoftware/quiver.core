package runtime

import (
	"context"
	"fmt"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/engines/wizard/runtime/models"
	"github.com/rabbytesoftware/quiver/internal/engines/wizard/runtime/process"
)

// Manager handles process tracking and lifecycle
type Manager struct {
	processes map[string]process.Process
	mu        sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		processes: make(map[string]process.Process),
	}
}

func (m *Manager) Register(proc process.Process) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processes[proc.ID()] = proc
}

func (m *Manager) Unregister(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.processes, id)
}

func (m *Manager) Get(id string) (process.Process, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	proc, exists := m.processes[id]
	if !exists {
		return nil, models.ErrProcessNotFound
	}
	return proc, nil
}

func (m *Manager) ListAll() []process.Process {
	m.mu.RLock()
	defer m.mu.RUnlock()

	procs := make([]process.Process, 0, len(m.processes))
	for _, proc := range m.processes {
		procs = append(procs, proc)
	}
	return procs
}

func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.processes)
}

func (m *Manager) ListByStatus(status models.Status) []process.Process {
	m.mu.RLock()
	defer m.mu.RUnlock()

	procs := make([]process.Process, 0)
	for _, proc := range m.processes {
		if proc.Status() == status {
			procs = append(procs, proc)
		}
	}
	return procs
}

func (m *Manager) StopAll(ctx context.Context) error {
	m.mu.RLock()
	runningProcs := make([]process.Process, 0)
	for _, proc := range m.processes {
		if proc.Status() == models.StatusRunning {
			runningProcs = append(runningProcs, proc)
		}
	}
	m.mu.RUnlock()

	var errs []error
	for _, proc := range runningProcs {
		if err := proc.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop process %s: %w", proc.ID(), err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("stop all completed with %d error(s): %v", len(errs), errs)
	}
	return nil
}

func (m *Manager) KillAll(ctx context.Context) error {
	m.mu.RLock()
	activeProcs := make([]process.Process, 0)
	for _, proc := range m.processes {
		if proc.Status().IsActive() {
			activeProcs = append(activeProcs, proc)
		}
	}
	m.mu.RUnlock()

	var errs []error
	for _, proc := range activeProcs {
		if err := proc.Kill(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to kill process %s: %w", proc.ID(), err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("kill all completed with %d error(s): %v", len(errs), errs)
	}
	return nil
}

func (m *Manager) CleanupFinished() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	finishedIDs := make([]string, 0)
	for id, proc := range m.processes {
		if proc.Status() == models.StatusFinished {
			if err := proc.Close(); err != nil {
				// Log error but continue cleanup
				// The error is non-critical as the process has already finished
			}
			finishedIDs = append(finishedIDs, id)
		}
	}

	for _, id := range finishedIDs {
		delete(m.processes, id)
	}

	return len(finishedIDs)
}

func (m *Manager) ShutdownAll(ctx context.Context) error {
	if err := m.StopAll(ctx); err != nil {
		if killErr := m.KillAll(ctx); killErr != nil {
			return fmt.Errorf("failed to stop gracefully and kill failed: %w (original: %v)", killErr, err)
		}
	}

	m.mu.Lock()
	ids := make([]string, 0, len(m.processes))
	for id := range m.processes {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	var errs []error
	for _, id := range ids {
		if proc, err := m.Get(id); err == nil {
			if closeErr := proc.Close(); closeErr != nil {
				errs = append(errs, fmt.Errorf("failed to close process %s: %w", id, closeErr))
			}
			m.Unregister(id)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown completed with %d error(s): %v", len(errs), errs)
	}
	return nil
}

// Clear removes all processes from tracking without stopping them
// WARNING: This should only be used for testing or when you're sure processes are already stopped
func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processes = make(map[string]process.Process)
}
