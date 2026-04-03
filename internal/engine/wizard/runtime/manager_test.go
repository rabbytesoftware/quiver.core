package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/core/watcher"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/mocks"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/models"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/process"
)

func TestMain(m *testing.M) {
	watcher.NewWatcherService()
	m.Run()
}

func TestNewManager(t *testing.T) {
	manager := NewManager()

	if manager == nil {
		t.Fatal("Expected manager to be created, got nil")
	}

	if manager.processes == nil {
		t.Error("Expected processes map to be initialized")
	}

	if manager.Count() != 0 {
		t.Errorf("Expected empty manager, got %d processes", manager.Count())
	}
}

func TestManager_Register(t *testing.T) {
	manager := NewManager()
	proc := mocks.NewProcess("test-1", models.StatusPrepared)

	manager.Register(proc)

	if manager.Count() != 1 {
		t.Errorf("Expected 1 process, got %d", manager.Count())
	}

	retrieved, err := manager.Get("test-1")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if retrieved.ID() != "test-1" {
		t.Errorf("Expected process ID 'test-1', got '%s'", retrieved.ID())
	}
}

func TestManager_RegisterMultiple(t *testing.T) {
	manager := NewManager()
	proc1 := mocks.NewProcess("test-1", models.StatusPrepared)
	proc2 := mocks.NewProcess("test-2", models.StatusRunning)
	proc3 := mocks.NewProcess("test-3", models.StatusFinished)

	manager.Register(proc1)
	manager.Register(proc2)
	manager.Register(proc3)

	if manager.Count() != 3 {
		t.Errorf("Expected 3 processes, got %d", manager.Count())
	}
}

func TestManager_RegisterDuplicate(t *testing.T) {
	manager := NewManager()
	proc1 := mocks.NewProcess("test-1", models.StatusPrepared)
	proc2 := mocks.NewProcess("test-1", models.StatusRunning)

	manager.Register(proc1)
	manager.Register(proc2) // Should overwrite

	if manager.Count() != 1 {
		t.Errorf("Expected 1 process, got %d", manager.Count())
	}

	retrieved, _ := manager.Get("test-1")
	if retrieved.Status() != models.StatusRunning {
		t.Errorf("Expected status %s, got %s", models.StatusRunning, retrieved.Status())
	}
}

func TestManager_Unregister(t *testing.T) {
	manager := NewManager()
	proc := mocks.NewProcess("test-1", models.StatusPrepared)

	manager.Register(proc)
	manager.Unregister("test-1")

	if manager.Count() != 0 {
		t.Errorf("Expected 0 processes, got %d", manager.Count())
	}

	_, err := manager.Get("test-1")
	if err == nil {
		t.Error("Expected error when getting unregistered process")
	}
}

func TestManager_UnregisterNonexistent(t *testing.T) {
	manager := NewManager()
	manager.Unregister("nonexistent") // Should not panic

	if manager.Count() != 0 {
		t.Errorf("Expected 0 processes, got %d", manager.Count())
	}
}

func TestManager_Get(t *testing.T) {
	manager := NewManager()
	proc := mocks.NewProcess("test-1", models.StatusPrepared)
	manager.Register(proc)

	retrieved, err := manager.Get("test-1")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if retrieved.ID() != "test-1" {
		t.Errorf("Expected process ID 'test-1', got '%s'", retrieved.ID())
	}
}

func TestManager_GetNonexistent(t *testing.T) {
	manager := NewManager()

	_, err := manager.Get("nonexistent")
	if err == nil {
		t.Error("Expected error when getting nonexistent process")
	}

	if !errors.Is(err, models.ErrProcessNotFound) {
		t.Errorf("Expected ErrProcessNotFound, got %v", err)
	}
}

func TestManager_ListAll(t *testing.T) {
	manager := NewManager()
	proc1 := mocks.NewProcess("test-1", models.StatusPrepared)
	proc2 := mocks.NewProcess("test-2", models.StatusRunning)
	proc3 := mocks.NewProcess("test-3", models.StatusFinished)

	manager.Register(proc1)
	manager.Register(proc2)
	manager.Register(proc3)

	procs := manager.ListAll()

	if len(procs) != 3 {
		t.Errorf("Expected 3 processes, got %d", len(procs))
	}
}

func TestManager_ListAllEmpty(t *testing.T) {
	manager := NewManager()
	procs := manager.ListAll()

	if len(procs) != 0 {
		t.Errorf("Expected 0 processes, got %d", len(procs))
	}

	if procs == nil {
		t.Error("Expected empty slice, got nil")
	}
}

func TestManager_Count(t *testing.T) {
	manager := NewManager()

	if manager.Count() != 0 {
		t.Errorf("Expected count 0, got %d", manager.Count())
	}

	proc1 := mocks.NewProcess("test-1", models.StatusPrepared)
	manager.Register(proc1)

	if manager.Count() != 1 {
		t.Errorf("Expected count 1, got %d", manager.Count())
	}

	proc2 := mocks.NewProcess("test-2", models.StatusRunning)
	manager.Register(proc2)

	if manager.Count() != 2 {
		t.Errorf("Expected count 2, got %d", manager.Count())
	}

	manager.Unregister("test-1")

	if manager.Count() != 1 {
		t.Errorf("Expected count 1, got %d", manager.Count())
	}
}

func TestManager_ListByStatus(t *testing.T) {
	manager := NewManager()
	proc1 := mocks.NewProcess("test-1", models.StatusPrepared)
	proc2 := mocks.NewProcess("test-2", models.StatusRunning)
	proc3 := mocks.NewProcess("test-3", models.StatusRunning)
	proc4 := mocks.NewProcess("test-4", models.StatusFinished)

	manager.Register(proc1)
	manager.Register(proc2)
	manager.Register(proc3)
	manager.Register(proc4)

	runningProcs := manager.ListByStatus(models.StatusRunning)
	if len(runningProcs) != 2 {
		t.Errorf("Expected 2 running processes, got %d", len(runningProcs))
	}

	preparedProcs := manager.ListByStatus(models.StatusPrepared)
	if len(preparedProcs) != 1 {
		t.Errorf("Expected 1 prepared process, got %d", len(preparedProcs))
	}

	finishedProcs := manager.ListByStatus(models.StatusFinished)
	if len(finishedProcs) != 1 {
		t.Errorf("Expected 1 finished process, got %d", len(finishedProcs))
	}
}

func TestManager_ListByStatusEmpty(t *testing.T) {
	manager := NewManager()
	procs := manager.ListByStatus(models.StatusRunning)

	if len(procs) != 0 {
		t.Errorf("Expected 0 processes, got %d", len(procs))
	}

	if procs == nil {
		t.Error("Expected empty slice, got nil")
	}
}

func TestManager_StopAll(t *testing.T) {
	manager := NewManager()
	proc1 := mocks.NewProcess("test-1", models.StatusRunning)
	proc2 := mocks.NewProcess("test-2", models.StatusRunning)
	proc3 := mocks.NewProcess("test-3", models.StatusFinished)

	manager.Register(proc1)
	manager.Register(proc2)
	manager.Register(proc3)

	ctx := context.Background()
	err := manager.StopAll(ctx)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify running processes were stopped
	if proc1.Status() != models.StatusFinished {
		t.Errorf("Expected proc1 to be finished, got %s", proc1.Status())
	}

	if proc2.Status() != models.StatusFinished {
		t.Errorf("Expected proc2 to be finished, got %s", proc2.Status())
	}

	// Finished process should remain unchanged
	if proc3.Status() != models.StatusFinished {
		t.Errorf("Expected proc3 to remain finished, got %s", proc3.Status())
	}
}

func TestManager_StopAllWithErrors(t *testing.T) {
	manager := NewManager()
	proc1 := mocks.NewProcess("test-1", models.StatusRunning)
	proc2 := mocks.NewProcess("test-2", models.StatusRunning)

	stopErr := errors.New("stop failed")
	proc1.StopErr = stopErr

	manager.Register(proc1)
	manager.Register(proc2)

	ctx := context.Background()
	err := manager.StopAll(ctx)

	if err == nil {
		t.Error("Expected error when stopping fails")
	}

	// proc2 should still be stopped despite proc1 failing
	if proc2.Status() != models.StatusFinished {
		t.Errorf("Expected proc2 to be finished, got %s", proc2.Status())
	}
}

func TestManager_StopAllEmpty(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	err := manager.StopAll(ctx)
	if err != nil {
		t.Errorf("Expected no error for empty manager, got %v", err)
	}
}

func TestManager_KillAll(t *testing.T) {
	manager := NewManager()
	proc1 := mocks.NewProcess("test-1", models.StatusRunning)
	proc2 := mocks.NewProcess("test-2", models.StatusStopping)
	proc3 := mocks.NewProcess("test-3", models.StatusFinished)

	manager.Register(proc1)
	manager.Register(proc2)
	manager.Register(proc3)

	ctx := context.Background()
	err := manager.KillAll(ctx)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Active processes should be killed
	if proc1.Status() != models.StatusFinished {
		t.Errorf("Expected proc1 to be finished, got %s", proc1.Status())
	}

	if proc2.Status() != models.StatusFinished {
		t.Errorf("Expected proc2 to be finished, got %s", proc2.Status())
	}

	// Finished process should remain unchanged
	if proc3.Status() != models.StatusFinished {
		t.Errorf("Expected proc3 to remain finished, got %s", proc3.Status())
	}
}

func TestManager_KillAllWithErrors(t *testing.T) {
	manager := NewManager()
	proc1 := mocks.NewProcess("test-1", models.StatusRunning)
	proc2 := mocks.NewProcess("test-2", models.StatusRunning)

	killErr := errors.New("kill failed")
	proc1.KillErr = killErr

	manager.Register(proc1)
	manager.Register(proc2)

	ctx := context.Background()
	err := manager.KillAll(ctx)

	if err == nil {
		t.Error("Expected error when killing fails")
	}

	// proc2 should still be killed despite proc1 failing
	if proc2.Status() != models.StatusFinished {
		t.Errorf("Expected proc2 to be finished, got %s", proc2.Status())
	}
}

func TestManager_KillAllEmpty(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	err := manager.KillAll(ctx)
	if err != nil {
		t.Errorf("Expected no error for empty manager, got %v", err)
	}
}

func TestManager_CleanupFinished(t *testing.T) {
	manager := NewManager()
	proc1 := mocks.NewProcess("test-1", models.StatusRunning)
	proc2 := mocks.NewProcess("test-2", models.StatusFinished)
	proc3 := mocks.NewProcess("test-3", models.StatusFinished)

	manager.Register(proc1)
	manager.Register(proc2)
	manager.Register(proc3)

	count := manager.CleanupFinished()

	if count != 2 {
		t.Errorf("Expected 2 cleaned up processes, got %d", count)
	}

	if manager.Count() != 1 {
		t.Errorf("Expected 1 remaining process, got %d", manager.Count())
	}

	// Verify only running process remains
	remaining := manager.ListAll()
	if len(remaining) != 1 || remaining[0].ID() != "test-1" {
		t.Error("Expected only test-1 to remain")
	}
}

func TestManager_CleanupFinishedEmpty(t *testing.T) {
	manager := NewManager()
	count := manager.CleanupFinished()

	if count != 0 {
		t.Errorf("Expected 0 cleaned up processes, got %d", count)
	}
}

func TestManager_CleanupFinishedWithCloseError(t *testing.T) {
	manager := NewManager()
	proc := mocks.NewProcess("test-1", models.StatusFinished)
	proc.CloseErr = errors.New("close failed")

	manager.Register(proc)

	// Should still cleanup despite close error
	count := manager.CleanupFinished()

	if count != 1 {
		t.Errorf("Expected 1 cleaned up process, got %d", count)
	}

	if manager.Count() != 0 {
		t.Errorf("Expected 0 remaining processes, got %d", manager.Count())
	}
}

func TestManager_ShutdownAll(t *testing.T) {
	manager := NewManager()
	proc1 := mocks.NewProcess("test-1", models.StatusRunning)
	proc2 := mocks.NewProcess("test-2", models.StatusRunning)
	proc3 := mocks.NewProcess("test-3", models.StatusFinished)

	manager.Register(proc1)
	manager.Register(proc2)
	manager.Register(proc3)

	ctx := context.Background()
	err := manager.ShutdownAll(ctx)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// All processes should be unregistered
	if manager.Count() != 0 {
		t.Errorf("Expected 0 processes after shutdown, got %d", manager.Count())
	}
}

func TestManager_ShutdownAllWithStopFailure(t *testing.T) {
	manager := NewManager()
	proc := mocks.NewProcess("test-1", models.StatusRunning)
	proc.StopErr = errors.New("stop failed")

	manager.Register(proc)

	ctx := context.Background()
	err := manager.ShutdownAll(ctx)

	// Should succeed because kill is attempted after stop fails
	if err != nil {
		t.Errorf("Expected no error after kill fallback, got %v", err)
	}

	// Process should still be unregistered
	if manager.Count() != 0 {
		t.Errorf("Expected 0 processes after shutdown, got %d", manager.Count())
	}
}

func TestManager_ShutdownAllWithCloseError(t *testing.T) {
	manager := NewManager()
	proc := mocks.NewProcess("test-1", models.StatusRunning)
	proc.CloseErr = errors.New("close failed")

	manager.Register(proc)

	ctx := context.Background()
	err := manager.ShutdownAll(ctx)

	if err == nil {
		t.Error("Expected error from close failure")
	}

	// Process should still be unregistered
	if manager.Count() != 0 {
		t.Errorf("Expected 0 processes after shutdown, got %d", manager.Count())
	}
}

func TestManager_ShutdownAllEmpty(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	err := manager.ShutdownAll(ctx)
	if err != nil {
		t.Errorf("Expected no error for empty manager, got %v", err)
	}
}

func TestManager_Clear(t *testing.T) {
	manager := NewManager()
	proc1 := mocks.NewProcess("test-1", models.StatusFinished)
	proc2 := mocks.NewProcess("test-2", models.StatusFinished)

	manager.Register(proc1)
	manager.Register(proc2)

	if err := manager.Clear(); err != nil {
		t.Fatalf("Expected no error clearing finished processes, got %v", err)
	}

	if manager.Count() != 0 {
		t.Errorf("Expected 0 processes after clear, got %d", manager.Count())
	}
}

func TestManager_Clear_WithActiveProcesses(t *testing.T) {
	manager := NewManager()
	proc1 := mocks.NewProcess("test-1", models.StatusRunning)
	proc2 := mocks.NewProcess("test-2", models.StatusFinished)

	manager.Register(proc1)
	manager.Register(proc2)

	if err := manager.Clear(); !errors.Is(err, models.ErrActiveProcesses) {
		t.Errorf("Expected ErrActiveProcesses, got %v", err)
	}

	// Map should be unchanged
	if manager.Count() != 2 {
		t.Errorf("Expected 2 processes after failed clear, got %d", manager.Count())
	}
}

func TestManager_ConcurrentAccess(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	var wg sync.WaitGroup
	numGoroutines := 10

	// Concurrent registrations
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			proc := mocks.NewProcess(string(rune('A'+id)), models.StatusPrepared)
			manager.Register(proc)
		}(i)
	}
	wg.Wait()

	if manager.Count() != numGoroutines {
		t.Errorf("Expected %d processes, got %d", numGoroutines, manager.Count())
	}

	// Concurrent reads
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_ = manager.ListAll()
			_ = manager.Count()
			_ = manager.ListByStatus(models.StatusPrepared)
		}()
	}
	wg.Wait()

	// Concurrent cleanup
	for i := 0; i < numGoroutines; i++ {
		proc := mocks.NewProcess(string(rune('Z'-i)), models.StatusFinished)
		manager.Register(proc)
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		manager.CleanupFinished()
	}()
	go func() {
		defer wg.Done()
		manager.ListAll()
	}()
	wg.Wait()

	// Concurrent shutdown
	for i := 0; i < numGoroutines; i++ {
		proc := mocks.NewProcess(string(rune('a'+i)), models.StatusRunning)
		manager.Register(proc)
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		manager.StopAll(ctx)
	}()
	go func() {
		defer wg.Done()
		manager.ListByStatus(models.StatusRunning)
	}()
	wg.Wait()
}

func TestManager_RegisterNilProcess(t *testing.T) {
	manager := NewManager()

	// Registering nil should panic (this is expected behavior)
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected Register to panic with nil process, but it didn't")
		}
	}()

	manager.Register(nil)
}

func TestManager_ProcessInterface(t *testing.T) {
	manager := NewManager()
	proc := mocks.NewProcess("test-1", models.StatusPrepared)

	// Verify that mockProcess correctly implements process.Process
	var _ process.Process = proc

	manager.Register(proc)

	retrieved, err := manager.Get("test-1")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Test all interface methods
	if retrieved.ID() != "test-1" {
		t.Errorf("Expected ID 'test-1', got '%s'", retrieved.ID())
	}

	if retrieved.Status() != models.StatusPrepared {
		t.Errorf("Expected status %s, got %s", models.StatusPrepared, retrieved.Status())
	}

	if retrieved.ExitCode() != 0 {
		t.Errorf("Expected exit code 0, got %d", retrieved.ExitCode())
	}

	if retrieved.Output() != "" {
		t.Errorf("Expected empty output, got '%s'", retrieved.Output())
	}

	if retrieved.Error() != "" {
		t.Errorf("Expected empty error, got '%s'", retrieved.Error())
	}
}

func TestManager_GetByKey(t *testing.T) {
	manager := NewManager()
	proc := mocks.NewProcess("test-1", models.StatusPrepared)
	manager.Register(proc)

	retrieved, err := manager.GetByKey(proc.Key())
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if retrieved.ID() != "test-1" {
		t.Errorf("Expected process ID 'test-1', got '%s'", retrieved.ID())
	}
}

func TestManager_GetByKey_Nonexistent(t *testing.T) {
	manager := NewManager()

	_, err := manager.GetByKey("nonexistent-key")
	if err == nil {
		t.Error("Expected error for nonexistent key")
	}
	if !errors.Is(err, models.ErrProcessNotFound) {
		t.Errorf("Expected ErrProcessNotFound, got %v", err)
	}
}

func TestManager_GetByKey_AfterUnregister(t *testing.T) {
	manager := NewManager()
	proc := mocks.NewProcess("test-1", models.StatusFinished)
	manager.Register(proc)
	manager.Unregister(proc.ID())

	_, err := manager.GetByKey(proc.Key())
	if !errors.Is(err, models.ErrProcessNotFound) {
		t.Errorf("Expected ErrProcessNotFound after unregister, got %v", err)
	}
}

func TestManager_GetByKey_AfterCleanupFinished(t *testing.T) {
	manager := NewManager()
	proc := mocks.NewProcess("test-1", models.StatusFinished)
	manager.Register(proc)
	manager.CleanupFinished()

	_, err := manager.GetByKey(proc.Key())
	if !errors.Is(err, models.ErrProcessNotFound) {
		t.Errorf("Expected ErrProcessNotFound after cleanup, got %v", err)
	}
}

func BenchmarkManager_GetByKey(b *testing.B) {
	for _, size := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			manager := NewManager()
			for i := 0; i < size; i++ {
				proc := mocks.NewProcess(fmt.Sprintf("proc-%d", i), models.StatusRunning)
				manager.Register(proc)
			}
			target := fmt.Sprintf("proc-%d", size-1)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = manager.GetByKey(target)
			}
		})
	}
}
