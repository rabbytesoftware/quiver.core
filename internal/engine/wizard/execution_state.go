package wizard

import (
	"context"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/process"
)

// executionState holds everything the wizard needs to track a running execution.
// A single instance is created per namespace at the start of Execute and
// deleted from the sync.Map when Execute returns.
//
// It also implements step.ProcessTracker so step handlers can record and read
// the OS process they manage, without coupling to wizard internals.
type executionState struct {
	cancel context.CancelFunc
	proc   process.Process
	mu     sync.RWMutex
}

func newExecutionState(
	cancel context.CancelFunc,
) *executionState {
	return &executionState{cancel: cancel}
}

// SetProcess records the running OS process.
// Called by the run step handler after a successful Start().
func (s *executionState) SetProcess(
	proc process.Process,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.proc = proc
}

// GetProcess returns the process and true if one has been set,
// or nil, false if no process has been started yet.
func (s *executionState) GetProcess() (process.Process, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.proc == nil {
		return nil, false
	}
	return s.proc, true
}
