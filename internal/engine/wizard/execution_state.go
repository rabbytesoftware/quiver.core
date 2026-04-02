package wizard

import (
	"context"
	"sync"
)

// executionState holds everything the wizard needs to track a running execution.
// A single instance is created per namespace at the start of Execute and
// deleted from the sync.Map when Execute returns.
//
// It also implements step.ProcessTracker so step handlers can record and read
// the key of the OS process they manage, without coupling to wizard internals.
type executionState struct {
	cancel     context.CancelFunc
	processKey string
	mu         sync.RWMutex
}

func newExecutionState(cancel context.CancelFunc) *executionState {
	return &executionState{cancel: cancel}
}

// SetKey records the key of the running OS process.
// Called by the run step handler after a successful Start().
func (s *executionState) SetKey(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processKey = key
}

// GetKey returns the process key and true if one has been set,
// or "", false if no process has been started yet.
func (s *executionState) GetKey() (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.processKey == "" {
		return "", false
	}
	return s.processKey, true
}
