package mocks

import "sync"

// Tracker is a test double for step.ProcessTracker.
type Tracker struct {
	mu  sync.RWMutex
	key string
	set bool
}

func (t *Tracker) SetKey(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.key = key
	t.set = true
}

func (t *Tracker) GetKey() (string, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.key, t.set
}
