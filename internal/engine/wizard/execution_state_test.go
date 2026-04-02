package wizard

import (
	"context"
	"sync"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/mocks"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/models"
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

func TestExecutionState_GetProcess_Empty(t *testing.T) {
	state := newExecutionState(func() {})

	proc, ok := state.GetProcess()
	if ok {
		t.Error("expected ok=false before SetProcess, got true")
	}
	if proc != nil {
		t.Errorf("expected nil process, got %v", proc)
	}
}

func TestExecutionState_SetProcess_GetProcess(t *testing.T) {
	state := newExecutionState(func() {})
	p := mocks.NewProcess("proc-abc-123", models.StatusRunning)
	state.SetProcess(p)

	proc, ok := state.GetProcess()
	if !ok {
		t.Error("expected ok=true after SetProcess, got false")
	}
	if proc != p {
		t.Errorf("expected stored process, got different value")
	}
}

func TestExecutionState_SetProcess_Overwrite(t *testing.T) {
	state := newExecutionState(func() {})
	first := mocks.NewProcess("first", models.StatusRunning)
	second := mocks.NewProcess("second", models.StatusRunning)
	state.SetProcess(first)
	state.SetProcess(second)

	proc, ok := state.GetProcess()
	if !ok || proc != second {
		t.Errorf("expected second process after overwrite")
	}
}

func TestExecutionState_ConcurrentAccess(t *testing.T) {
	state := newExecutionState(func() {})
	p := mocks.NewProcess("key", models.StatusRunning)

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			state.SetProcess(p)
		}()
		go func() {
			defer wg.Done()
			state.GetProcess()
		}()
	}
	wg.Wait()
}

func TestExecutionState_ImplementsProcessTracker(t *testing.T) {
	state := newExecutionState(func() {})
	p := mocks.NewProcess("test", models.StatusRunning)
	state.SetProcess(p)

	proc, ok := state.GetProcess()
	if !ok || proc != p {
		t.Errorf("ProcessTracker contract not satisfied")
	}
}
