//go:build darwin

package process

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/infrastructure/runtime/models"
)

func TestNewDarwinProcess(t *testing.T) {
	config := models.NewConfig([]string{"echo", "test"})
	ctx := context.Background()

	proc, err := NewDarwinProcess(ctx, config)
	if err != nil {
		t.Fatalf("NewDarwinProcess() error = %v", err)
	}

	if proc == nil {
		t.Fatal("NewDarwinProcess() returned nil")
	}

	if proc.BaseProcess == nil {
		t.Error("BaseProcess should not be nil")
	}

	proc.Close()
}

func TestDarwinProcess_Start(t *testing.T) {
	config := models.NewConfig([]string{"echo", "Hello Darwin"})
	ctx := context.Background()

	proc, _ := NewDarwinProcess(ctx, config)
	defer proc.Close()

	err := proc.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if proc.Status() != models.StatusRunning && proc.Status() != models.StatusFinished {
		t.Errorf("Status after Start() = %v, want Running or Finished", proc.Status())
	}

	// Wait for completion
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	proc.Wait(ctx)

	output := proc.Output()
	if !strings.Contains(output, "Hello Darwin") {
		t.Errorf("Output = %q, should contain 'Hello Darwin'", output)
	}
}

func TestDarwinProcess_Start_AlreadyStarted(t *testing.T) {
	config := models.NewConfig([]string{"sleep", "10"})
	ctx := context.Background()

	proc, _ := NewDarwinProcess(ctx, config)
	defer proc.Close()
	defer proc.Kill(context.Background())

	err := proc.Start(ctx)
	if err != nil {
		t.Fatalf("First Start() error = %v", err)
	}

	// Try to start again
	err = proc.Start(ctx)
	if err != models.ErrInvalidState {
		t.Errorf("Second Start() error = %v, want %v", err, models.ErrInvalidState)
	}
}

func TestDarwinProcess_Stop(t *testing.T) {
	config := models.NewConfig([]string{"sleep", "30"})
	ctx := context.Background()

	proc, _ := NewDarwinProcess(ctx, config)
	defer proc.Close()

	err := proc.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Give process time to actually start
	time.Sleep(100 * time.Millisecond)

	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = proc.Stop(stopCtx)
	if err != nil {
		// In CI/sandboxed environments, signaling process groups may fail with "operation not permitted"
		// This functionality is thoroughly tested in Manager tests (TestManager_StopAll)
		if err.Error() == "failed to stop: operation not permitted" {
			t.Skip("Skipping test due to environment permissions (tested via Manager tests)")
		}
		t.Errorf("Stop() error = %v", err)
	}

	if proc.Status() != models.StatusFinished {
		t.Errorf("Status after Stop() = %v, want %v", proc.Status(), models.StatusFinished)
	}
}

func TestDarwinProcess_Stop_NotRunning(t *testing.T) {
	config := models.NewConfig([]string{"echo", "test"})
	ctx := context.Background()

	proc, _ := NewDarwinProcess(ctx, config)
	defer proc.Close()

	err := proc.Stop(ctx)
	if err != models.ErrInvalidState {
		t.Errorf("Stop() on not running process error = %v, want %v", err, models.ErrInvalidState)
	}
}

func TestDarwinProcess_Kill(t *testing.T) {
	config := models.NewConfig([]string{"sleep", "30"})
	ctx := context.Background()

	proc, _ := NewDarwinProcess(ctx, config)
	defer proc.Close()

	err := proc.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Give process time to start
	time.Sleep(100 * time.Millisecond)

	killCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = proc.Kill(killCtx)
	if err != nil {
		// In CI/sandboxed environments, killing process groups may fail with "operation not permitted"
		// This functionality is thoroughly tested in Manager tests (TestManager_KillAll)
		if err.Error() == "failed to kill: operation not permitted" {
			t.Skip("Skipping test due to environment permissions (tested via Manager tests)")
		}
		t.Errorf("Kill() error = %v", err)
	}

	if proc.Status() != models.StatusFinished {
		t.Errorf("Status after Kill() = %v, want %v", proc.Status(), models.StatusFinished)
	}
}

func TestDarwinProcess_OutputStreaming(t *testing.T) {
	config := models.NewConfig([]string{"sh", "-c", "echo line1; echo line2; echo line3"})
	ctx := context.Background()

	proc, _ := NewDarwinProcess(ctx, config)
	defer proc.Close()

	// Collect output from stream
	var streamOutput []string
	var mu sync.Mutex
	done := make(chan struct{})

	go func() {
		defer close(done)
		for line := range proc.StreamOutput() {
			mu.Lock()
			streamOutput = append(streamOutput, line)
			mu.Unlock()
		}
	}()

	err := proc.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Wait for completion
	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	proc.Wait(waitCtx)

	// Wait for streaming goroutine to finish
	<-done

	mu.Lock()
	lineCount := len(streamOutput)
	mu.Unlock()

	if lineCount != 3 {
		t.Errorf("streamed %d lines, want 3", lineCount)
	}

	// Check buffered output contains all lines
	output := proc.Output()
	if !strings.Contains(output, "line1") || !strings.Contains(output, "line2") || !strings.Contains(output, "line3") {
		t.Errorf("Output = %q, should contain line1, line2, and line3", output)
	}
}

func TestDarwinProcess_ErrorStreaming(t *testing.T) {
	config := models.NewConfig([]string{"sh", "-c", "echo error1 >&2; echo error2 >&2"})
	ctx := context.Background()

	proc, _ := NewDarwinProcess(ctx, config)
	defer proc.Close()

	// Collect error from stream
	var streamError []string
	var mu sync.Mutex
	done := make(chan struct{})

	go func() {
		defer close(done)
		for line := range proc.StreamError() {
			mu.Lock()
			streamError = append(streamError, line)
			mu.Unlock()
		}
	}()

	err := proc.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Wait for completion
	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	proc.Wait(waitCtx)

	// Wait for streaming goroutine to finish
	<-done

	mu.Lock()
	lineCount := len(streamError)
	mu.Unlock()

	if lineCount != 2 {
		t.Errorf("streamed %d error lines, want 2", lineCount)
	}

	// Check buffered error contains all lines
	errOutput := proc.Error()
	if !strings.Contains(errOutput, "error1") || !strings.Contains(errOutput, "error2") {
		t.Errorf("Error = %q, should contain error1 and error2", errOutput)
	}
}

func TestDarwinProcess_ExitCode(t *testing.T) {
	tests := []struct {
		name     string
		command  []string
		wantCode int
	}{
		{
			name:     "successful command",
			command:  []string{"sh", "-c", "exit 0"},
			wantCode: 0,
		},
		{
			name:     "failed command",
			command:  []string{"sh", "-c", "exit 1"},
			wantCode: 1,
		},
		{
			name:     "custom exit code",
			command:  []string{"sh", "-c", "exit 42"},
			wantCode: 42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := models.NewConfig(tt.command)
			ctx := context.Background()

			proc, _ := NewDarwinProcess(ctx, config)
			defer proc.Close()

			proc.Start(ctx)

			waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			proc.Wait(waitCtx)

			if proc.ExitCode() != tt.wantCode {
				t.Errorf("ExitCode = %d, want %d", proc.ExitCode(), tt.wantCode)
			}
		})
	}
}

func TestDarwinProcess_NewWithError(t *testing.T) {
	// Test NewDarwinProcess with empty command
	config := models.NewConfig([]string{})
	ctx := context.Background()

	proc, err := NewDarwinProcess(ctx, config)
	if err != models.ErrEmptyCommand {
		t.Errorf("NewDarwinProcess() error = %v, want %v", err, models.ErrEmptyCommand)
	}

	if proc != nil {
		t.Error("NewDarwinProcess() should return nil for empty command")
		proc.Close()
	}
}
