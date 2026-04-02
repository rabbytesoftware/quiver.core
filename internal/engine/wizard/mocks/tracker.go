package mocks

import (
	"sync"

	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/process"
)

// Tracker is a test double for step.ProcessTracker.
type Tracker struct {
	mu   sync.RWMutex
	proc process.Process
}

func (t *Tracker) SetProcess(proc process.Process) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.proc = proc
}

func (t *Tracker) GetProcess() (process.Process, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.proc == nil {
		return nil, false
	}
	return t.proc, true
}
