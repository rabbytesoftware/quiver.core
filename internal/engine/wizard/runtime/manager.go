package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/models"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/process"
)

// Manager handles process tracking and lifecycle
type Manager struct {
	processes map[string]process.Process
	keyIndex  map[string]string // process key -> process ID
	mu        sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		processes: make(map[string]process.Process),
		keyIndex:  make(map[string]string),
	}
}

func (m *Manager) Register(proc process.Process) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processes[proc.ID()] = proc
	// Key may be empty if the process hasn't started yet; index lazily in GetByKey.
	if key := proc.Key(); key != "" {
		m.keyIndex[key] = proc.ID()
	}
}

func (m *Manager) Unregister(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if proc, exists := m.processes[id]; exists {
		delete(m.keyIndex, proc.Key())
	}
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

func (m *Manager) GetByKey(key string) (process.Process, error) {
	// Fast path: check the index first.
	m.mu.RLock()
	if id, exists := m.keyIndex[key]; exists {
		proc := m.processes[id]
		m.mu.RUnlock()
		if proc != nil {
			return proc, nil
		}
	} else {
		m.mu.RUnlock()
	}

	// Slow path: linear scan + populate index for next lookup.
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, proc := range m.processes {
		if proc.Key() == key {
			m.keyIndex[key] = proc.ID()
			return proc, nil
		}
	}
	return nil, models.ErrProcessNotFound
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
				slog.Warn("failed to close finished process", "id", id, "error", err)
			}
			finishedIDs = append(finishedIDs, id)
		}
	}

	for _, id := range finishedIDs {
		if proc, exists := m.processes[id]; exists {
			delete(m.keyIndex, proc.Key())
		}
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

// Clear removes all processes from tracking without stopping them.
// Returns ErrActiveProcesses if any process is still running, stopping, or killing.
// WARNING: This should only be used for testing or when you're sure processes are already stopped.
func (m *Manager) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, proc := range m.processes {
		if proc.Status().IsActive() {
			return models.ErrActiveProcesses
		}
	}

	m.processes = make(map[string]process.Process)
	m.keyIndex = make(map[string]string)
	return nil
}
