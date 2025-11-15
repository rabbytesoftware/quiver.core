package runtime

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/infrastructure/runtime/models"
	"github.com/rabbytesoftware/quiver/internal/infrastructure/runtime/process"
)

func TestNewManager(t *testing.T) {
	manager := NewManager()

	if manager == nil {
		t.Fatal("NewManager() returned nil")
	}

	if manager.processes == nil {
		t.Error("processes map not initialized")
	}

	if manager.Count() != 0 {
		t.Errorf("Count() = %d, want 0", manager.Count())
	}
}

func TestManager_Register(t *testing.T) {
	manager := NewManager()
	config := models.NewConfig([]string{"echo", "test"})
	ctx := context.Background()

	var proc process.Process
	var err error

	switch runtime.GOOS {
	case "darwin":
		proc, err = process.NewDarwinProcess(ctx, config)
	case "linux":
		proc, err = process.NewLinuxProcess(ctx, config)
	case "windows":
		proc, err = process.NewWindowsProcess(ctx, config)
	default:
		t.Skip("Unsupported OS")
	}

	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}
	defer proc.Close()

	manager.Register(proc)

	if manager.Count() != 1 {
		t.Errorf("Count() = %d, want 1", manager.Count())
	}

	retrieved, err := manager.Get(proc.ID())
	if err != nil {
		t.Errorf("Get() error = %v", err)
	}

	if retrieved.ID() != proc.ID() {
		t.Error("Retrieved process does not match registered process")
	}
}

func TestManager_Unregister(t *testing.T) {
	manager := NewManager()
	config := models.NewConfig([]string{"echo", "test"})
	ctx := context.Background()

	var proc process.Process
	var err error

	switch runtime.GOOS {
	case "darwin":
		proc, err = process.NewDarwinProcess(ctx, config)
	case "linux":
		proc, err = process.NewLinuxProcess(ctx, config)
	case "windows":
		proc, err = process.NewWindowsProcess(ctx, config)
	default:
		t.Skip("Unsupported OS")
	}

	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}
	defer proc.Close()

	manager.Register(proc)

	if manager.Count() != 1 {
		t.Error("Process not registered")
	}

	manager.Unregister(proc.ID())

	if manager.Count() != 0 {
		t.Errorf("Count() = %d, want 0 after unregister", manager.Count())
	}

	_, err = manager.Get(proc.ID())
	if err != models.ErrProcessNotFound {
		t.Errorf("Get() after unregister error = %v, want %v", err, models.ErrProcessNotFound)
	}
}

func TestManager_Get(t *testing.T) {
	manager := NewManager()
	config := models.NewConfig([]string{"echo", "test"})
	ctx := context.Background()

	var proc process.Process
	var err error

	switch runtime.GOOS {
	case "darwin":
		proc, err = process.NewDarwinProcess(ctx, config)
	case "linux":
		proc, err = process.NewLinuxProcess(ctx, config)
	case "windows":
		proc, err = process.NewWindowsProcess(ctx, config)
	default:
		t.Skip("Unsupported OS")
	}

	if err != nil {
		t.Fatalf("Failed to create process: %v", err)
	}
	defer proc.Close()

	manager.Register(proc)

	retrieved, err := manager.Get(proc.ID())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if retrieved == nil {
		t.Fatal("Get() returned nil")
	}

	if retrieved.ID() != proc.ID() {
		t.Error("Retrieved process ID does not match")
	}
}

func TestManager_Get_NotFound(t *testing.T) {
	manager := NewManager()

	_, err := manager.Get("non-existent-id")
	if err != models.ErrProcessNotFound {
		t.Errorf("Get() error = %v, want %v", err, models.ErrProcessNotFound)
	}
}

func TestManager_ListAll(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	var proc1, proc2, proc3 process.Process
	var err error

	// Register multiple processes
	switch runtime.GOOS {
	case "darwin":
		proc1, err = process.NewDarwinProcess(ctx, models.NewConfig([]string{"echo", "1"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc2, err = process.NewDarwinProcess(ctx, models.NewConfig([]string{"echo", "2"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc3, err = process.NewDarwinProcess(ctx, models.NewConfig([]string{"echo", "3"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
	case "linux":
		proc1, err = process.NewLinuxProcess(ctx, models.NewConfig([]string{"echo", "1"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc2, err = process.NewLinuxProcess(ctx, models.NewConfig([]string{"echo", "2"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc3, err = process.NewLinuxProcess(ctx, models.NewConfig([]string{"echo", "3"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
	case "windows":
		proc1, err = process.NewWindowsProcess(ctx, models.NewConfig([]string{"cmd", "/c", "echo 1"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc2, err = process.NewWindowsProcess(ctx, models.NewConfig([]string{"cmd", "/c", "echo 2"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc3, err = process.NewWindowsProcess(ctx, models.NewConfig([]string{"cmd", "/c", "echo 3"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
	default:
		t.Skip("Unsupported OS")
	}

	defer proc1.Close()
	defer proc2.Close()
	defer proc3.Close()

	manager.Register(proc1)
	manager.Register(proc2)
	manager.Register(proc3)

	list := manager.ListAll()
	if len(list) != 3 {
		t.Errorf("ListAll() length = %d, want 3", len(list))
	}

	// Verify all processes are in the list
	ids := make(map[string]bool)
	for _, proc := range list {
		ids[proc.ID()] = true
	}

	if !ids[proc1.ID()] || !ids[proc2.ID()] || !ids[proc3.ID()] {
		t.Error("Not all processes found in ListAll()")
	}
}

func TestManager_ListAll_Empty(t *testing.T) {
	manager := NewManager()

	list := manager.ListAll()
	if len(list) != 0 {
		t.Errorf("ListAll() on empty manager length = %d, want 0", len(list))
	}
}

func TestManager_Count(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	if manager.Count() != 0 {
		t.Error("Initial count should be 0")
	}

	var proc1, proc2 process.Process
	var err error

	switch runtime.GOOS {
	case "darwin":
		proc1, err = process.NewDarwinProcess(ctx, models.NewConfig([]string{"echo", "1"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc2, err = process.NewDarwinProcess(ctx, models.NewConfig([]string{"echo", "2"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
	case "linux":
		proc1, err = process.NewLinuxProcess(ctx, models.NewConfig([]string{"echo", "1"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc2, err = process.NewLinuxProcess(ctx, models.NewConfig([]string{"echo", "2"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
	case "windows":
		proc1, err = process.NewWindowsProcess(ctx, models.NewConfig([]string{"cmd", "/c", "echo 1"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc2, err = process.NewWindowsProcess(ctx, models.NewConfig([]string{"cmd", "/c", "echo 2"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
	default:
		t.Skip("Unsupported OS")
	}

	defer proc1.Close()
	defer proc2.Close()

	manager.Register(proc1)

	if manager.Count() != 1 {
		t.Errorf("Count() = %d, want 1", manager.Count())
	}

	manager.Register(proc2)

	if manager.Count() != 2 {
		t.Errorf("Count() = %d, want 2", manager.Count())
	}

	manager.Unregister(proc1.ID())

	if manager.Count() != 1 {
		t.Errorf("Count() after unregister = %d, want 1", manager.Count())
	}
}

func TestManager_ListByStatus(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	var proc1, proc2, proc3 process.Process
	var err error

	switch runtime.GOOS {
	case "darwin":
		proc1, err = process.NewDarwinProcess(ctx, models.NewConfig([]string{"echo", "1"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc2, err = process.NewDarwinProcess(ctx, models.NewConfig([]string{"sleep", "30"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc3, err = process.NewDarwinProcess(ctx, models.NewConfig([]string{"sleep", "30"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
	case "linux":
		proc1, err = process.NewLinuxProcess(ctx, models.NewConfig([]string{"echo", "1"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc2, err = process.NewLinuxProcess(ctx, models.NewConfig([]string{"sleep", "30"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc3, err = process.NewLinuxProcess(ctx, models.NewConfig([]string{"sleep", "30"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
	case "windows":
		proc1, err = process.NewWindowsProcess(ctx, models.NewConfig([]string{"cmd", "/c", "echo 1"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc2, err = process.NewWindowsProcess(ctx, models.NewConfig([]string{"timeout", "/t", "30", "/nobreak"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc3, err = process.NewWindowsProcess(ctx, models.NewConfig([]string{"timeout", "/t", "30", "/nobreak"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
	default:
		t.Skip("Unsupported OS")
	}

	// Start proc2 and proc3 to set them to running status
	proc2.Start(ctx)
	proc3.Start(ctx)
	
	// Give processes time to actually start running
	time.Sleep(100 * time.Millisecond)

	manager.Register(proc1)
	manager.Register(proc2)
	manager.Register(proc3)

	runningProcs := manager.ListByStatus(models.StatusRunning)
	if len(runningProcs) != 2 {
		t.Errorf("ListByStatus(Running) length = %d, want 2", len(runningProcs))
	}

	preparedProcs := manager.ListByStatus(models.StatusPrepared)
	if len(preparedProcs) != 1 {
		t.Errorf("ListByStatus(Prepared) length = %d, want 1", len(preparedProcs))
	}

	finishedProcs := manager.ListByStatus(models.StatusFinished)
	if len(finishedProcs) != 0 {
		t.Errorf("ListByStatus(Finished) length = %d, want 0", len(finishedProcs))
	}

	// Stop and wait for processes to complete before they're cleaned up
	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	proc2.Stop(stopCtx)
	proc3.Stop(stopCtx)
	
	// Wait for processes to finish
	proc2.Wait(stopCtx)
	proc3.Wait(stopCtx)
	
	// Now safe to close
	proc1.Close()
	proc2.Close()
	proc3.Close()
}

func TestManager_StopAll(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping StopAll test on Windows (different signal handling)")
	}

	manager := NewManager()
	ctx := context.Background()

	// Start a few processes
	proc1, _ := process.NewDarwinProcess(ctx, models.NewConfig([]string{"sleep", "30"}))
	proc2, _ := process.NewDarwinProcess(ctx, models.NewConfig([]string{"sleep", "30"}))

	manager.Register(proc1)
	manager.Register(proc2)

	proc1.Start(ctx)
	proc2.Start(ctx)

	// Give processes time to start
	time.Sleep(100 * time.Millisecond)

	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := manager.StopAll(stopCtx)
	if err != nil {
		t.Errorf("StopAll() error = %v", err)
	}

	// Verify both processes are stopped
	if proc1.Status() != models.StatusFinished {
		t.Errorf("proc1 status = %v, want Finished", proc1.Status())
	}

	if proc2.Status() != models.StatusFinished {
		t.Errorf("proc2 status = %v, want Finished", proc2.Status())
	}

	proc1.Close()
	proc2.Close()
}

func TestManager_KillAll(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	var proc1, proc2 process.Process

	switch runtime.GOOS {
	case "darwin":
		proc1, _ = process.NewDarwinProcess(ctx, models.NewConfig([]string{"sleep", "30"}))
		proc2, _ = process.NewDarwinProcess(ctx, models.NewConfig([]string{"sleep", "30"}))
	case "linux":
		proc1, _ = process.NewLinuxProcess(ctx, models.NewConfig([]string{"sleep", "30"}))
		proc2, _ = process.NewLinuxProcess(ctx, models.NewConfig([]string{"sleep", "30"}))
	case "windows":
		proc1, _ = process.NewWindowsProcess(ctx, models.NewConfig([]string{"cmd", "/c", "timeout /t 30"}))
		proc2, _ = process.NewWindowsProcess(ctx, models.NewConfig([]string{"cmd", "/c", "timeout /t 30"}))
	default:
		t.Skip("Unsupported OS")
	}

	manager.Register(proc1)
	manager.Register(proc2)

	proc1.Start(ctx)
	proc2.Start(ctx)

	// Give processes time to start
	time.Sleep(100 * time.Millisecond)

	killCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := manager.KillAll(killCtx)
	if err != nil {
		t.Errorf("KillAll() error = %v", err)
	}

	// Verify both processes are finished
	if proc1.Status() != models.StatusFinished {
		t.Errorf("proc1 status = %v, want Finished", proc1.Status())
	}

	if proc2.Status() != models.StatusFinished {
		t.Errorf("proc2 status = %v, want Finished", proc2.Status())
	}

	proc1.Close()
	proc2.Close()
}

func TestManager_CleanupFinished(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	var proc1, proc2, proc3 process.Process
	var err error

	switch runtime.GOOS {
	case "darwin":
		proc1, err = process.NewDarwinProcess(ctx, models.NewConfig([]string{"echo", "1"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc2, err = process.NewDarwinProcess(ctx, models.NewConfig([]string{"sleep", "30"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc3, err = process.NewDarwinProcess(ctx, models.NewConfig([]string{"echo", "3"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
	case "linux":
		proc1, err = process.NewLinuxProcess(ctx, models.NewConfig([]string{"echo", "1"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc2, err = process.NewLinuxProcess(ctx, models.NewConfig([]string{"sleep", "30"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc3, err = process.NewLinuxProcess(ctx, models.NewConfig([]string{"echo", "3"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
	case "windows":
		proc1, err = process.NewWindowsProcess(ctx, models.NewConfig([]string{"cmd", "/c", "echo 1"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc2, err = process.NewWindowsProcess(ctx, models.NewConfig([]string{"cmd", "/c", "timeout /t 30"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc3, err = process.NewWindowsProcess(ctx, models.NewConfig([]string{"cmd", "/c", "echo 3"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
	default:
		t.Skip("Unsupported OS")
	}

	defer proc2.Close()

	// Start and wait for proc1 and proc3 to finish
	proc1.Start(ctx)
	proc3.Start(ctx)
	proc1.Wait(ctx)
	proc3.Wait(ctx)

	// Start proc2 (but don't wait - it's a long-running sleep)
	proc2.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	manager.Register(proc1)
	manager.Register(proc2)
	manager.Register(proc3)

	if manager.Count() != 3 {
		t.Error("All 3 processes should be registered")
	}

	cleanedCount := manager.CleanupFinished()
	if cleanedCount != 2 {
		t.Errorf("CleanupFinished() = %d, want 2", cleanedCount)
	}

	if manager.Count() != 1 {
		t.Errorf("Count() after cleanup = %d, want 1", manager.Count())
	}

	// Verify only running process remains
	remaining := manager.ListAll()
	if len(remaining) != 1 || remaining[0].ID() != proc2.ID() {
		t.Error("Only running process should remain")
	}
}

func TestManager_ShutdownAll(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	var proc1, proc2 process.Process

	switch runtime.GOOS {
	case "darwin":
		proc1, _ = process.NewDarwinProcess(ctx, models.NewConfig([]string{"echo", "test1"}))
		proc2, _ = process.NewDarwinProcess(ctx, models.NewConfig([]string{"echo", "test2"}))
	case "linux":
		proc1, _ = process.NewLinuxProcess(ctx, models.NewConfig([]string{"echo", "test1"}))
		proc2, _ = process.NewLinuxProcess(ctx, models.NewConfig([]string{"echo", "test2"}))
	case "windows":
		proc1, _ = process.NewWindowsProcess(ctx, models.NewConfig([]string{"cmd", "/c", "echo test1"}))
		proc2, _ = process.NewWindowsProcess(ctx, models.NewConfig([]string{"cmd", "/c", "echo test2"}))
	default:
		t.Skip("Unsupported OS")
	}

	manager.Register(proc1)
	manager.Register(proc2)

	proc1.Start(ctx)
	proc2.Start(ctx)

	// Give processes time to complete
	time.Sleep(200 * time.Millisecond)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := manager.ShutdownAll(shutdownCtx)
	if err != nil {
		t.Errorf("ShutdownAll() error = %v", err)
	}

	if manager.Count() != 0 {
		t.Errorf("Count() after shutdown = %d, want 0", manager.Count())
	}
}

func TestManager_Clear(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	var proc1, proc2 process.Process
	var err error

	switch runtime.GOOS {
	case "darwin":
		proc1, err = process.NewDarwinProcess(ctx, models.NewConfig([]string{"echo", "1"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc2, err = process.NewDarwinProcess(ctx, models.NewConfig([]string{"echo", "2"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
	case "linux":
		proc1, err = process.NewLinuxProcess(ctx, models.NewConfig([]string{"echo", "1"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc2, err = process.NewLinuxProcess(ctx, models.NewConfig([]string{"echo", "2"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
	case "windows":
		proc1, err = process.NewWindowsProcess(ctx, models.NewConfig([]string{"cmd", "/c", "echo 1"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
		proc2, err = process.NewWindowsProcess(ctx, models.NewConfig([]string{"cmd", "/c", "echo 2"}))
		if err != nil {
			t.Fatalf("Failed to create process: %v", err)
		}
	default:
		t.Skip("Unsupported OS")
	}

	manager.Register(proc1)
	manager.Register(proc2)

	if manager.Count() != 2 {
		t.Error("Both processes should be registered")
	}

	manager.Clear()

	if manager.Count() != 0 {
		t.Errorf("Count() after Clear() = %d, want 0", manager.Count())
	}

	list := manager.ListAll()
	if len(list) != 0 {
		t.Error("ListAll() should be empty after Clear()")
	}
}

func TestManager_ConcurrentAccess(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	done := make(chan bool)
	numGoroutines := 10
	procsPerGoroutine := 5

	// Concurrent register
	for i := 0; i < numGoroutines; i++ {
		go func() {
			for j := 0; j < procsPerGoroutine; j++ {
				var proc process.Process
				var err error

				switch runtime.GOOS {
				case "darwin":
					proc, err = process.NewDarwinProcess(ctx, models.NewConfig([]string{"echo", "test"}))
				case "linux":
					proc, err = process.NewLinuxProcess(ctx, models.NewConfig([]string{"echo", "test"}))
				case "windows":
					proc, err = process.NewWindowsProcess(ctx, models.NewConfig([]string{"cmd", "/c", "echo test"}))
				default:
					continue
				}

				if err == nil {
					manager.Register(proc)
				}
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	expectedCount := numGoroutines * procsPerGoroutine
	if manager.Count() != expectedCount {
		t.Errorf("Count() = %d, want %d", manager.Count(), expectedCount)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		go func() {
			_ = manager.ListAll()
			_ = manager.Count()
			done <- true
		}()
	}

	// Wait for read goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

