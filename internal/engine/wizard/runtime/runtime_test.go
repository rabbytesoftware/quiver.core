package runtime

import (
	"context"
	"errors"
	"os"
	"runtime"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/models"
)

func TestNew(t *testing.T) {
	rt, err := New()

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if rt == nil {
		t.Fatal("Expected runtime to be created, got nil")
	}

	if rt.manager == nil {
		t.Error("Expected manager to be initialized")
	}

	if rt.os == "" {
		t.Error("Expected OS to be detected")
	}
}

func TestNew_DetectedOS(t *testing.T) {
	rt, err := New()

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	expectedOS := runtime.GOOS
	if rt.os != expectedOS {
		t.Errorf("Expected OS '%s', got '%s'", expectedOS, rt.os)
	}
}

func TestNew_SupportedOS(t *testing.T) {
	// This test verifies that the current OS is supported
	rt, err := New()

	if err != nil {
		t.Fatalf("Expected no error on supported OS, got %v", err)
	}

	supported := []string{"darwin", "linux", "windows"}
	isSupported := false
	for _, os := range supported {
		if rt.os == os {
			isSupported = true
			break
		}
	}

	if !isSupported {
		t.Errorf("Current OS '%s' should be supported", rt.os)
	}
}

func TestRuntime_OS(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	detectedOS := rt.OS()
	expectedOS := runtime.GOOS

	if detectedOS != expectedOS {
		t.Errorf("Expected OS '%s', got '%s'", expectedOS, detectedOS)
	}
}

func TestRuntime_Get(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	ctx := context.Background()
	builder := rt.Get(ctx, "echo", "hello")

	if builder == nil {
		t.Fatal("Expected builder to be created, got nil")
	}
}

func TestRuntime_GetEmpty(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	ctx := context.Background()
	builder := rt.Get(ctx)

	if builder == nil {
		t.Fatal("Expected builder to be created even with empty command, got nil")
	}
}

func TestRuntime_GetByID(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Register a mock process
	proc := newMockProcess("test-1", models.StatusPrepared)
	rt.manager.Register(proc)

	retrieved, err := rt.GetByID("test-1")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if retrieved.ID() != "test-1" {
		t.Errorf("Expected process ID 'test-1', got '%s'", retrieved.ID())
	}
}

func TestRuntime_GetByIDNonexistent(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	_, err = rt.GetByID("nonexistent")
	if err == nil {
		t.Error("Expected error when getting nonexistent process")
	}

	if !errors.Is(err, models.ErrProcessNotFound) {
		t.Errorf("Expected ErrProcessNotFound, got %v", err)
	}
}

func TestRuntime_ListAll(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Initially should be empty
	procs := rt.ListAll()
	if len(procs) != 0 {
		t.Errorf("Expected 0 processes, got %d", len(procs))
	}

	// Add some processes
	proc1 := newMockProcess("test-1", models.StatusPrepared)
	proc2 := newMockProcess("test-2", models.StatusRunning)
	proc3 := newMockProcess("test-3", models.StatusFinished)

	rt.manager.Register(proc1)
	rt.manager.Register(proc2)
	rt.manager.Register(proc3)

	procs = rt.ListAll()
	if len(procs) != 3 {
		t.Errorf("Expected 3 processes, got %d", len(procs))
	}
}

func TestRuntime_ListByStatus(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	proc1 := newMockProcess("test-1", models.StatusPrepared)
	proc2 := newMockProcess("test-2", models.StatusRunning)
	proc3 := newMockProcess("test-3", models.StatusRunning)
	proc4 := newMockProcess("test-4", models.StatusFinished)

	rt.manager.Register(proc1)
	rt.manager.Register(proc2)
	rt.manager.Register(proc3)
	rt.manager.Register(proc4)

	runningProcs := rt.ListByStatus(models.StatusRunning)
	if len(runningProcs) != 2 {
		t.Errorf("Expected 2 running processes, got %d", len(runningProcs))
	}

	preparedProcs := rt.ListByStatus(models.StatusPrepared)
	if len(preparedProcs) != 1 {
		t.Errorf("Expected 1 prepared process, got %d", len(preparedProcs))
	}

	finishedProcs := rt.ListByStatus(models.StatusFinished)
	if len(finishedProcs) != 1 {
		t.Errorf("Expected 1 finished process, got %d", len(finishedProcs))
	}
}

func TestRuntime_Count(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if rt.Count() != 0 {
		t.Errorf("Expected count 0, got %d", rt.Count())
	}

	proc1 := newMockProcess("test-1", models.StatusPrepared)
	rt.manager.Register(proc1)

	if rt.Count() != 1 {
		t.Errorf("Expected count 1, got %d", rt.Count())
	}

	proc2 := newMockProcess("test-2", models.StatusRunning)
	rt.manager.Register(proc2)

	if rt.Count() != 2 {
		t.Errorf("Expected count 2, got %d", rt.Count())
	}

	rt.manager.Unregister("test-1")

	if rt.Count() != 1 {
		t.Errorf("Expected count 1, got %d", rt.Count())
	}
}

func TestRuntime_StopAll(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	proc1 := newMockProcess("test-1", models.StatusRunning)
	proc2 := newMockProcess("test-2", models.StatusRunning)
	rt.manager.Register(proc1)
	rt.manager.Register(proc2)

	ctx := context.Background()
	err = rt.StopAll(ctx)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if proc1.Status() != models.StatusFinished {
		t.Errorf("Expected proc1 to be finished, got %s", proc1.Status())
	}

	if proc2.Status() != models.StatusFinished {
		t.Errorf("Expected proc2 to be finished, got %s", proc2.Status())
	}
}

func TestRuntime_StopAllEmpty(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	ctx := context.Background()
	err = rt.StopAll(ctx)

	if err != nil {
		t.Errorf("Expected no error for empty runtime, got %v", err)
	}
}

func TestRuntime_KillAll(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	proc1 := newMockProcess("test-1", models.StatusRunning)
	proc2 := newMockProcess("test-2", models.StatusRunning)
	rt.manager.Register(proc1)
	rt.manager.Register(proc2)

	ctx := context.Background()
	err = rt.KillAll(ctx)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if proc1.Status() != models.StatusFinished {
		t.Errorf("Expected proc1 to be finished, got %s", proc1.Status())
	}

	if proc2.Status() != models.StatusFinished {
		t.Errorf("Expected proc2 to be finished, got %s", proc2.Status())
	}
}

func TestRuntime_KillAllEmpty(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	ctx := context.Background()
	err = rt.KillAll(ctx)

	if err != nil {
		t.Errorf("Expected no error for empty runtime, got %v", err)
	}
}

func TestRuntime_CleanupFinished(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	proc1 := newMockProcess("test-1", models.StatusRunning)
	proc2 := newMockProcess("test-2", models.StatusFinished)
	proc3 := newMockProcess("test-3", models.StatusFinished)

	rt.manager.Register(proc1)
	rt.manager.Register(proc2)
	rt.manager.Register(proc3)

	count := rt.CleanupFinished()

	if count != 2 {
		t.Errorf("Expected 2 cleaned up processes, got %d", count)
	}

	if rt.Count() != 1 {
		t.Errorf("Expected 1 remaining process, got %d", rt.Count())
	}
}

func TestRuntime_CleanupFinishedEmpty(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	count := rt.CleanupFinished()

	if count != 0 {
		t.Errorf("Expected 0 cleaned up processes, got %d", count)
	}
}

func TestRuntime_Shutdown(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	proc1 := newMockProcess("test-1", models.StatusRunning)
	proc2 := newMockProcess("test-2", models.StatusRunning)
	rt.manager.Register(proc1)
	rt.manager.Register(proc2)

	ctx := context.Background()
	err = rt.Shutdown(ctx)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// All processes should be unregistered after shutdown
	if rt.Count() != 0 {
		t.Errorf("Expected 0 processes after shutdown, got %d", rt.Count())
	}
}

func TestRuntime_ShutdownEmpty(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	ctx := context.Background()
	err = rt.Shutdown(ctx)

	if err != nil {
		t.Errorf("Expected no error for empty runtime, got %v", err)
	}
}

func TestDetectOS(t *testing.T) {
	detected := detectOS()
	expected := runtime.GOOS

	if detected != expected {
		t.Errorf("Expected OS '%s', got '%s'", expected, detected)
	}
}

func TestIsSupportedOS(t *testing.T) {
	tests := []struct {
		name     string
		os       string
		expected bool
	}{
		{"Darwin", "darwin", true},
		{"Linux", "linux", true},
		{"Windows", "windows", true},
		{"FreeBSD", "freebsd", false},
		{"OpenBSD", "openbsd", false},
		{"Solaris", "solaris", false},
		{"AIX", "aix", false},
		{"Plan9", "plan9", false},
		{"Empty", "", false},
		{"Invalid", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSupportedOS(tt.os)
			if result != tt.expected {
				t.Errorf("isSupportedOS(%s) = %v, expected %v", tt.os, result, tt.expected)
			}
		})
	}
}

func TestRuntime_Integration(t *testing.T) {
	// Skip if not on a supported OS
	if !isSupportedOS(runtime.GOOS) {
		t.Skipf("Skipping integration test on unsupported OS: %s", runtime.GOOS)
	}

	rt, err := New()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify initial state
	if rt.Count() != 0 {
		t.Errorf("Expected 0 processes initially, got %d", rt.Count())
	}

	// Register some test processes
	proc1 := newMockProcess("test-1", models.StatusPrepared)
	proc2 := newMockProcess("test-2", models.StatusRunning)
	proc3 := newMockProcess("test-3", models.StatusFinished)

	rt.manager.Register(proc1)
	rt.manager.Register(proc2)
	rt.manager.Register(proc3)

	// Verify registration
	if rt.Count() != 3 {
		t.Errorf("Expected 3 processes, got %d", rt.Count())
	}

	// List by status
	runningProcs := rt.ListByStatus(models.StatusRunning)
	if len(runningProcs) != 1 {
		t.Errorf("Expected 1 running process, got %d", len(runningProcs))
	}

	// Get by ID
	retrieved, err := rt.GetByID("test-2")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if retrieved.ID() != "test-2" {
		t.Errorf("Expected process ID 'test-2', got '%s'", retrieved.ID())
	}

	// Cleanup finished
	cleanedCount := rt.CleanupFinished()
	if cleanedCount != 1 {
		t.Errorf("Expected 1 cleaned process, got %d", cleanedCount)
	}
	if rt.Count() != 2 {
		t.Errorf("Expected 2 processes after cleanup, got %d", rt.Count())
	}

	// Stop all
	ctx := context.Background()
	err = rt.StopAll(ctx)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify running process was stopped
	if proc2.Status() != models.StatusFinished {
		t.Errorf("Expected proc2 to be finished, got %s", proc2.Status())
	}

	// Shutdown
	err = rt.Shutdown(ctx)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify all processes are unregistered
	if rt.Count() != 0 {
		t.Errorf("Expected 0 processes after shutdown, got %d", rt.Count())
	}
}

func TestRuntime_ContextCancellation(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	proc := newMockProcess("test-1", models.StatusRunning)
	rt.manager.Register(proc)

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Operations should handle cancelled context gracefully
	_ = rt.StopAll(ctx)
	// Error might be returned depending on mock implementation
	// The important thing is that it doesn't panic

	_ = rt.KillAll(ctx)
	// Same as above

	_ = rt.Shutdown(ctx)
	// Same as above
}

func TestRuntime_GetBuilder(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	tests := []struct {
		name    string
		command []string
	}{
		{"SingleCommand", []string{"echo"}},
		{"CommandWithArgs", []string{"echo", "hello", "world"}},
		{"EmptyCommand", []string{}},
		{"ComplexCommand", []string{"bash", "-c", "echo hello && echo world"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			builder := rt.Get(ctx, tt.command...)

			if builder == nil {
				t.Fatal("Expected builder to be created, got nil")
			}
		})
	}
}

func TestRuntime_OSEnvironmentVariable(t *testing.T) {
	// Save original value
	originalGOOS := os.Getenv("GOOS")
	defer func() {
		if originalGOOS != "" {
			os.Setenv("GOOS", originalGOOS)
		} else {
			os.Unsetenv("GOOS")
		}
	}()

	// Test that detectOS uses runtime.GOOS, not environment variable
	os.Setenv("GOOS", "unsupported")
	detected := detectOS()

	// Should still detect the actual runtime GOOS
	if detected != runtime.GOOS {
		t.Errorf("Expected OS '%s', got '%s'", runtime.GOOS, detected)
	}
}

func TestRuntime_ManagerIndependence(t *testing.T) {
	rt1, err := New()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	rt2, err := New()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Register processes in rt1
	proc1 := newMockProcess("test-1", models.StatusRunning)
	rt1.manager.Register(proc1)

	// rt2 should be independent
	if rt1.Count() != 1 {
		t.Errorf("Expected rt1 count 1, got %d", rt1.Count())
	}

	if rt2.Count() != 0 {
		t.Errorf("Expected rt2 count 0, got %d", rt2.Count())
	}

	// Register in rt2
	proc2 := newMockProcess("test-2", models.StatusRunning)
	rt2.manager.Register(proc2)

	// Both should maintain their own processes
	if rt1.Count() != 1 {
		t.Errorf("Expected rt1 count 1, got %d", rt1.Count())
	}

	if rt2.Count() != 1 {
		t.Errorf("Expected rt2 count 1, got %d", rt2.Count())
	}

	// Get operations should be independent
	_, err = rt1.GetByID("test-1")
	if err != nil {
		t.Error("rt1 should have test-1")
	}

	_, err = rt1.GetByID("test-2")
	if err == nil {
		t.Error("rt1 should not have test-2")
	}

	_, err = rt2.GetByID("test-2")
	if err != nil {
		t.Error("rt2 should have test-2")
	}

	_, err = rt2.GetByID("test-1")
	if err == nil {
		t.Error("rt2 should not have test-1")
	}
}

func TestRuntime_MultipleOperations(t *testing.T) {
	rt, err := New()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	ctx := context.Background()

	// Perform multiple operations in sequence
	proc1 := newMockProcess("test-1", models.StatusRunning)
	proc2 := newMockProcess("test-2", models.StatusRunning)
	proc3 := newMockProcess("test-3", models.StatusFinished)

	rt.manager.Register(proc1)
	rt.manager.Register(proc2)
	rt.manager.Register(proc3)

	// First operation: cleanup finished
	count := rt.CleanupFinished()
	if count != 1 {
		t.Errorf("Expected 1 cleaned process, got %d", count)
	}

	// Second operation: list all
	procs := rt.ListAll()
	if len(procs) != 2 {
		t.Errorf("Expected 2 processes, got %d", len(procs))
	}

	// Third operation: stop all
	err = rt.StopAll(ctx)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Fourth operation: count
	count = rt.Count()
	if count != 2 {
		t.Errorf("Expected 2 processes, got %d", count)
	}

	// Fifth operation: cleanup again
	cleanedCount := rt.CleanupFinished()
	if cleanedCount != 2 {
		t.Errorf("Expected 2 cleaned processes, got %d", cleanedCount)
	}

	// Final operation: verify empty
	if rt.Count() != 0 {
		t.Errorf("Expected 0 processes, got %d", rt.Count())
	}
}
