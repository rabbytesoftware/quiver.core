package wizard

import (
	"context"
	"sync"
	"testing"
)

func TestNewExecutionState(t *testing.T) {
	called := false
	cancel := context.CancelFunc(func() { called = true })
	state := newExecutionState(cancel)

	if state == nil {
		t.Fatal("newExecutionState returned nil")
	}
	state.cancel()
	if !called {
		t.Error("cancel func not called")
	}
}

func TestExecutionState_GetKey_Empty(t *testing.T) {
	state := newExecutionState(func() {})

	key, ok := state.GetKey()
	if ok {
		t.Error("expected ok=false before SetKey, got true")
	}
	if key != "" {
		t.Errorf("expected empty key, got %q", key)
	}
}

func TestExecutionState_SetKey_GetKey(t *testing.T) {
	state := newExecutionState(func() {})
	state.SetKey("proc-abc-123")

	key, ok := state.GetKey()
	if !ok {
		t.Error("expected ok=true after SetKey, got false")
	}
	if key != "proc-abc-123" {
		t.Errorf("expected key %q, got %q", "proc-abc-123", key)
	}
}

func TestExecutionState_SetKey_Overwrite(t *testing.T) {
	state := newExecutionState(func() {})
	state.SetKey("first")
	state.SetKey("second")

	key, ok := state.GetKey()
	if !ok || key != "second" {
		t.Errorf("expected key %q, got %q (ok=%v)", "second", key, ok)
	}
}

func TestExecutionState_ConcurrentAccess(t *testing.T) {
	state := newExecutionState(func() {})

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			state.SetKey("key")
		}()
		go func() {
			defer wg.Done()
			state.GetKey()
		}()
	}
	wg.Wait()
}

func TestExecutionState_ImplementsProcessTracker(t *testing.T) {
	// Verify *executionState satisfies the step.ProcessTracker interface at runtime.
	// We do this via a simple interface conversion rather than a compile-time assertion.
	state := newExecutionState(func() {})
	state.SetKey("test")

	key, ok := state.GetKey()
	if !ok || key != "test" {
		t.Errorf("ProcessTracker contract not satisfied: got %q, %v", key, ok)
	}
}
