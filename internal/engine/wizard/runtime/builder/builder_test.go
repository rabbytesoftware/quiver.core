package builder

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/models"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/process"
)

// MockManager for testing
type MockManager struct {
	registered   []process.Process
	unregistered []string
}

func (m *MockManager) Register(p process.Process) {
	m.registered = append(m.registered, p)
}

func (m *MockManager) Unregister(id string) {
	m.unregistered = append(m.unregistered, id)
}

func TestNewBuilder(t *testing.T) {
	ctx := context.Background()
	manager := &MockManager{}
	command := []string{"echo", "test"}

	builder := NewBuilder(ctx, manager, "darwin", command)

	if builder == nil {
		t.Fatal("NewBuilder() returned nil")
	}

	if builder.ctx != ctx {
		t.Error("Builder context not set correctly")
	}

	if builder.manager == nil {
		t.Error("Builder manager should not be nil")
	}

	if builder.os != "darwin" {
		t.Errorf("Builder os = %s, want darwin", builder.os)
	}

	if len(builder.config.Command) != len(command) {
		t.Errorf("Builder command length = %d, want %d", len(builder.config.Command), len(command))
	}
}

func TestBuilder_WithWorkDir(t *testing.T) {
	ctx := context.Background()
	manager := &MockManager{}
	builder := NewBuilder(ctx, manager, "darwin", []string{"echo"})

	result := builder.WithWorkDir("/tmp")

	if result != builder {
		t.Error("WithWorkDir() should return same builder for chaining")
	}

	if builder.config.WorkDir != "/tmp" {
		t.Errorf("WorkDir = %s, want /tmp", builder.config.WorkDir)
	}
}

func TestBuilder_WithEnv(t *testing.T) {
	ctx := context.Background()
	manager := &MockManager{}
	builder := NewBuilder(ctx, manager, "darwin", []string{"echo"})

	env := map[string]string{
		"KEY1": "value1",
		"KEY2": "value2",
	}

	result := builder.WithEnv(env)

	if result != builder {
		t.Error("WithEnv() should return same builder for chaining")
	}

	for k, v := range env {
		if builder.config.Env[k] != v {
			t.Errorf("Env[%s] = %s, want %s", k, builder.config.Env[k], v)
		}
	}
}

func TestBuilder_WithEnvVar(t *testing.T) {
	ctx := context.Background()
	manager := &MockManager{}
	builder := NewBuilder(ctx, manager, "darwin", []string{"echo"})

	result := builder.WithEnvVar("TEST_VAR", "test_value")

	if result != builder {
		t.Error("WithEnvVar() should return same builder for chaining")
	}

	if builder.config.Env["TEST_VAR"] != "test_value" {
		t.Errorf("Env[TEST_VAR] = %s, want test_value", builder.config.Env["TEST_VAR"])
	}
}

func TestBuilder_WithTimeout(t *testing.T) {
	ctx := context.Background()
	manager := &MockManager{}
	builder := NewBuilder(ctx, manager, "darwin", []string{"echo"})

	timeout := 30 * time.Second
	result := builder.WithTimeout(timeout)

	if result != builder {
		t.Error("WithTimeout() should return same builder for chaining")
	}

	if builder.config.Timeout != timeout {
		t.Errorf("Timeout = %v, want %v", builder.config.Timeout, timeout)
	}
}

func TestBuilder_WithBufferSize(t *testing.T) {
	ctx := context.Background()
	manager := &MockManager{}
	builder := NewBuilder(ctx, manager, "darwin", []string{"echo"})

	size := 1024
	result := builder.WithBufferSize(size)

	if result != builder {
		t.Error("WithBufferSize() should return same builder for chaining")
	}

	if builder.config.BufferSize != size {
		t.Errorf("BufferSize = %d, want %d", builder.config.BufferSize, size)
	}
}

func TestBuilder_FluentChaining(t *testing.T) {
	ctx := context.Background()
	manager := &MockManager{}

	builder := NewBuilder(ctx, manager, "darwin", []string{"echo", "test"}).
		WithWorkDir("/tmp").
		WithEnv(map[string]string{"KEY1": "value1"}).
		WithEnvVar("KEY2", "value2").
		WithTimeout(30 * time.Second).
		WithBufferSize(512)

	if builder.config.WorkDir != "/tmp" {
		t.Error("WorkDir not set correctly in fluent chain")
	}

	if builder.config.Env["KEY1"] != "value1" {
		t.Error("KEY1 not set correctly in fluent chain")
	}

	if builder.config.Env["KEY2"] != "value2" {
		t.Error("KEY2 not set correctly in fluent chain")
	}

	if builder.config.Timeout != 30*time.Second {
		t.Error("Timeout not set correctly in fluent chain")
	}

	if builder.config.BufferSize != 512 {
		t.Error("BufferSize not set correctly in fluent chain")
	}
}

func TestBuilder_Build_CurrentOS(t *testing.T) {
	ctx := context.Background()
	manager := &MockManager{}

	var osName string
	switch runtime.GOOS {
	case "darwin":
		osName = "darwin"
	case "linux":
		osName = "linux"
	case "windows":
		osName = "windows"
	default:
		t.Skip("Unsupported OS for this test")
	}

	builder := NewBuilder(ctx, manager, osName, []string{"echo", "test"})
	proc, err := builder.Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if proc == nil {
		t.Fatal("Build() returned nil process")
	}

	// Verify process was registered
	if len(manager.registered) != 1 {
		t.Errorf("registered count = %d, want 1", len(manager.registered))
	}

	if manager.registered[0] != proc {
		t.Error("registered process does not match returned process")
	}

	// Clean up
	proc.Close()
}

func TestBuilder_Build_Darwin(t *testing.T) {
	ctx := context.Background()
	manager := &MockManager{}
	builder := NewBuilder(ctx, manager, "darwin", []string{"echo", "test"})

	proc, err := builder.Build()

	if runtime.GOOS == "darwin" {
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if proc == nil {
			t.Fatal("Build() returned nil process")
		}
		proc.Close()
	} else {
		if err != models.ErrUnsupportedOS {
			t.Errorf("Build() error = %v, want %v", err, models.ErrUnsupportedOS)
		}
	}
}

func TestBuilder_Build_Linux(t *testing.T) {
	ctx := context.Background()
	manager := &MockManager{}
	builder := NewBuilder(ctx, manager, "linux", []string{"echo", "test"})

	proc, err := builder.Build()

	if runtime.GOOS == "linux" {
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if proc == nil {
			t.Fatal("Build() returned nil process")
		}
		proc.Close()
	} else {
		if err != models.ErrUnsupportedOS {
			t.Errorf("Build() error = %v, want %v", err, models.ErrUnsupportedOS)
		}
	}
}

func TestBuilder_Build_Windows(t *testing.T) {
	ctx := context.Background()
	manager := &MockManager{}
	builder := NewBuilder(ctx, manager, "windows", []string{"echo", "test"})

	proc, err := builder.Build()

	if runtime.GOOS == "windows" {
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if proc == nil {
			t.Fatal("Build() returned nil process")
		}
		proc.Close()
	} else {
		if err != models.ErrUnsupportedOS {
			t.Errorf("Build() error = %v, want %v", err, models.ErrUnsupportedOS)
		}
	}
}

func TestBuilder_Build_UnsupportedOS(t *testing.T) {
	ctx := context.Background()
	manager := &MockManager{}
	builder := NewBuilder(ctx, manager, "unsupported", []string{"echo", "test"})

	proc, err := builder.Build()

	if err != models.ErrUnsupportedOS {
		t.Errorf("Build() error = %v, want %v", err, models.ErrUnsupportedOS)
	}

	if proc != nil {
		t.Error("Build() should return nil for unsupported OS")
	}
}

func TestBuilder_Build_EmptyCommand(t *testing.T) {
	ctx := context.Background()
	manager := &MockManager{}
	builder := NewBuilder(ctx, manager, "darwin", []string{})

	proc, err := builder.Build()

	if err != models.ErrEmptyCommand {
		t.Errorf("Build() error = %v, want %v", err, models.ErrEmptyCommand)
	}

	if proc != nil {
		t.Error("Build() should return nil for empty command")
	}
}

func TestBuilder_Build_WithAllOptions(t *testing.T) {
	ctx := context.Background()
	manager := &MockManager{}

	proc, err := NewBuilder(ctx, manager, runtime.GOOS, []string{"echo", "test"}).
		WithWorkDir("/tmp").
		WithEnv(map[string]string{"KEY": "value"}).
		WithTimeout(10 * time.Second).
		WithBufferSize(256).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if proc == nil {
		t.Fatal("Build() returned nil")
	}

	// Verify process is registered
	if len(manager.registered) != 1 {
		t.Errorf("registered count = %d, want 1", len(manager.registered))
	}

	proc.Close()
}

func TestBuilder_MultipleBuilds(t *testing.T) {
	ctx := context.Background()
	manager := &MockManager{}

	builder := NewBuilder(ctx, manager, runtime.GOOS, []string{"echo", "test"})

	proc1, err1 := builder.Build()
	if err1 != nil {
		t.Fatalf("First Build() error = %v", err1)
	}

	proc2, err2 := builder.Build()
	if err2 != nil {
		t.Fatalf("Second Build() error = %v", err2)
	}

	// Should create different processes
	if proc1.ID() == proc2.ID() {
		t.Error("Multiple builds should create unique processes")
	}

	// Both should be registered
	if len(manager.registered) != 2 {
		t.Errorf("registered count = %d, want 2", len(manager.registered))
	}

	proc1.Close()
	proc2.Close()
}

func TestBuilder_WithKillTimeout(t *testing.T) {
	ctx := context.Background()
	manager := &MockManager{}
	builder := NewBuilder(ctx, manager, "darwin", []string{"echo"})

	timeout := 5 * time.Second
	result := builder.WithKillTimeout(timeout)

	if result != builder {
		t.Error("WithKillTimeout() should return same builder for chaining")
	}

	if builder.config.KillTimeout != timeout {
		t.Errorf("KillTimeout = %v, want %v", builder.config.KillTimeout, timeout)
	}
}

func TestBuilder_WithStopTimeout(t *testing.T) {
	ctx := context.Background()
	manager := &MockManager{}
	builder := NewBuilder(ctx, manager, "darwin", []string{"echo"})

	timeout := 10 * time.Second
	result := builder.WithStopTimeout(timeout)

	if result != builder {
		t.Error("WithStopTimeout() should return same builder for chaining")
	}

	if builder.config.StopTimeout != timeout {
		t.Errorf("StopTimeout = %v, want %v", builder.config.StopTimeout, timeout)
	}
}

func TestBuilder_WithAllTimeouts(t *testing.T) {
	ctx := context.Background()
	manager := &MockManager{}

	builder := NewBuilder(ctx, manager, runtime.GOOS, []string{"echo", "test"}).
		WithTimeout(30 * time.Second).
		WithKillTimeout(5 * time.Second).
		WithStopTimeout(10 * time.Second)

	if builder.config.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", builder.config.Timeout)
	}

	if builder.config.KillTimeout != 5*time.Second {
		t.Errorf("KillTimeout = %v, want 5s", builder.config.KillTimeout)
	}

	if builder.config.StopTimeout != 10*time.Second {
		t.Errorf("StopTimeout = %v, want 10s", builder.config.StopTimeout)
	}
}

func TestBuilder_ChainWithTimeouts(t *testing.T) {
	ctx := context.Background()
	manager := &MockManager{}

	proc, err := NewBuilder(ctx, manager, runtime.GOOS, []string{"echo", "test"}).
		WithWorkDir("/tmp").
		WithTimeout(30 * time.Second).
		WithKillTimeout(5 * time.Second).
		WithStopTimeout(10 * time.Second).
		WithBufferSize(512).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if proc == nil {
		t.Fatal("Build() returned nil")
	}

	proc.Close()
}
