package process

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/models"
)

func TestNewBaseProcess(t *testing.T) {
	config := models.NewConfig([]string{"echo", "test"})
	ctx := context.Background()

	proc, err := NewBaseProcess(ctx, config)
	if err != nil {
		t.Fatalf("NewBaseProcess() error = %v", err)
	}

	if proc == nil {
		t.Fatal("NewBaseProcess() returned nil")
	}

	if proc.ID() == "" {
		t.Error("Process ID should not be empty")
	}

	if proc.Status() != models.StatusPrepared {
		t.Errorf("Status = %v, want %v", proc.Status(), models.StatusPrepared)
	}

	if proc.ExitCode() != -1 {
		t.Errorf("ExitCode = %d, want -1", proc.ExitCode())
	}
}

func TestNewBaseProcess_EmptyCommand(t *testing.T) {
	config := models.NewConfig([]string{})
	ctx := context.Background()

	proc, err := NewBaseProcess(ctx, config)
	if err != models.ErrEmptyCommand {
		t.Errorf("NewBaseProcess() error = %v, want %v", err, models.ErrEmptyCommand)
	}

	if proc != nil {
		t.Error("NewBaseProcess() should return nil for empty command")
	}
}

func TestNewBaseProcess_WithBufferSize(t *testing.T) {
	config := models.NewConfig([]string{"echo", "test"})
	config.BufferSize = 500
	ctx := context.Background()

	proc, err := NewBaseProcess(ctx, config)
	if err != nil {
		t.Fatalf("NewBaseProcess() error = %v", err)
	}

	if cap(proc.StreamOutput()) != 500 {
		t.Errorf("StreamOutput capacity = %d, want 500", cap(proc.StreamOutput()))
	}
}

func TestBaseProcess_ID(t *testing.T) {
	config := models.NewConfig([]string{"echo", "test"})
	ctx := context.Background()

	proc1, _ := NewBaseProcess(ctx, config)
	proc2, _ := NewBaseProcess(ctx, config)

	if proc1.ID() == proc2.ID() {
		t.Error("Process IDs should be unique")
	}

	if len(proc1.ID()) == 0 {
		t.Error("Process ID should not be empty")
	}
}

func TestBaseProcess_Status(t *testing.T) {
	config := models.NewConfig([]string{"echo", "test"})
	ctx := context.Background()

	proc, _ := NewBaseProcess(ctx, config)

	if proc.Status() != models.StatusPrepared {
		t.Errorf("Initial status = %v, want %v", proc.Status(), models.StatusPrepared)
	}

	// Test SetStatus
	proc.SetStatus(models.StatusRunning)
	if proc.Status() != models.StatusRunning {
		t.Errorf("Status after SetStatus = %v, want %v", proc.Status(), models.StatusRunning)
	}
}

func TestBaseProcess_ExitCode(t *testing.T) {
	config := models.NewConfig([]string{"echo", "test"})
	ctx := context.Background()

	proc, _ := NewBaseProcess(ctx, config)

	if proc.ExitCode() != -1 {
		t.Errorf("Initial exit code = %d, want -1", proc.ExitCode())
	}

	// Test SetExitCode
	proc.SetExitCode(0)
	if proc.ExitCode() != 0 {
		t.Errorf("ExitCode after SetExitCode = %d, want 0", proc.ExitCode())
	}

	proc.SetExitCode(1)
	if proc.ExitCode() != 1 {
		t.Errorf("ExitCode after SetExitCode = %d, want 1", proc.ExitCode())
	}
}

func TestBaseProcess_Wait(t *testing.T) {
	config := models.NewConfig([]string{"echo", "test"})
	ctx := context.Background()

	proc, _ := NewBaseProcess(ctx, config)

	// Close doneChan to simulate completion
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(proc.doneChan)
	}()

	err := proc.Wait(context.Background())
	if err != nil {
		t.Errorf("Wait() error = %v, want nil", err)
	}
}

func TestBaseProcess_Wait_Timeout(t *testing.T) {
	config := models.NewConfig([]string{"echo", "test"})
	ctx := context.Background()

	proc, _ := NewBaseProcess(ctx, config)

	// Don't close doneChan, just timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := proc.Wait(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("Wait() error = %v, want %v", err, context.DeadlineExceeded)
	}
}

func TestBaseProcess_Close(t *testing.T) {
	config := models.NewConfig([]string{"echo", "test"})
	ctx := context.Background()

	proc, _ := NewBaseProcess(ctx, config)

	err := proc.Close()
	if err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}

	// Verify handler is closed
	if !proc.outputHandler.IsClosed() {
		t.Error("OutputHandler should be closed after Close()")
	}
}

func TestEnvMapToSlice(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want int
	}{
		{
			name: "nil map",
			env:  nil,
			want: 0,
		},
		{
			name: "empty map",
			env:  map[string]string{},
			want: 0,
		},
		{
			name: "single entry",
			env:  map[string]string{"KEY1": "value1"},
			want: 1,
		},
		{
			name: "multiple entries",
			env: map[string]string{
				"KEY1": "value1",
				"KEY2": "value2",
				"KEY3": "value3",
			},
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := envMapToSlice(tt.env)

			if tt.env == nil {
				if result != nil {
					t.Errorf("envMapToSlice(nil) = %v, want nil", result)
				}
				return
			}

			if len(result) != tt.want {
				t.Errorf("envMapToSlice() length = %d, want %d", len(result), tt.want)
			}

			// Verify format
			for _, entry := range result {
				if !strings.Contains(entry, "=") {
					t.Errorf("entry %q should contain '='", entry)
				}
			}

			// Verify all keys are present
			for key, value := range tt.env {
				expected := key + "=" + value
				found := false
				for _, entry := range result {
					if entry == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected entry %q not found in result", expected)
				}
			}
		})
	}
}
