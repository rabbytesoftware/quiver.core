//go:build linux

package process

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/models"
)

func TestNewLinuxProcess(t *testing.T) {
	config := models.NewConfig([]string{"echo", "test"})
	ctx := context.Background()

	proc, err := NewLinuxProcess(ctx, config)
	if err != nil {
		t.Fatalf("NewLinuxProcess() error = %v", err)
	}

	if proc == nil {
		t.Fatal("NewLinuxProcess() returned nil")
	}

	if proc.BaseProcess == nil {
		t.Error("BaseProcess should not be nil")
	}

	proc.Close()
}

func TestLinuxProcess_Start_AlreadyStarted(t *testing.T) {
	config := models.NewConfig([]string{"sleep", "10"})
	ctx := context.Background()

	proc, _ := NewLinuxProcess(ctx, config)
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

func TestLinuxProcess_Stop(t *testing.T) {
	config := models.NewConfig([]string{"sleep", "30"})
	ctx := context.Background()

	proc, _ := NewLinuxProcess(ctx, config)
	defer proc.Close()

	err := proc.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Give process time to actually start
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	ticker := time.NewTicker(1 * time.Millisecond)
	defer ticker.Stop()

	for {
		if proc.Status() == models.StatusRunning {
			break
		}
		select {
		case <-ctx.Done():
			break
		case <-ticker.C:
		}
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = proc.Stop(stopCtx)
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	if proc.Status() != models.StatusFinished {
		t.Errorf("Status after Stop() = %v, want %v", proc.Status(), models.StatusFinished)
	}
}

func TestLinuxProcess_Stop_NotRunning(t *testing.T) {
	config := models.NewConfig([]string{"echo", "test"})
	ctx := context.Background()

	proc, _ := NewLinuxProcess(ctx, config)
	defer proc.Close()

	err := proc.Stop(ctx)
	if err != models.ErrInvalidState {
		t.Errorf("Stop() on not running process error = %v, want %v", err, models.ErrInvalidState)
	}
}

func TestLinuxProcess_Kill(t *testing.T) {
	config := models.NewConfig([]string{"sleep", "30"})
	ctx := context.Background()

	proc, _ := NewLinuxProcess(ctx, config)
	defer proc.Close()

	err := proc.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Wait for process to start
	pollCtx, pollCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer pollCancel()
	pollTicker := time.NewTicker(1 * time.Millisecond)
	defer pollTicker.Stop()

	for {
		if proc.Status() == models.StatusRunning {
			break
		}
		select {
		case <-pollCtx.Done():
			break
		case <-pollTicker.C:
		}
	}

	killCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = proc.Kill(killCtx)
	if err != nil {
		t.Errorf("Kill() error = %v", err)
	}

	if proc.Status() != models.StatusFinished {
		t.Errorf("Status after Kill() = %v, want %v", proc.Status(), models.StatusFinished)
	}
}

func TestLinuxProcess_ErrorStreaming(t *testing.T) {
	config := models.NewConfig([]string{"sh", "-c", "echo error1 >&2; echo error2 >&2"})
	ctx := context.Background()

	proc, _ := NewLinuxProcess(ctx, config)
	defer proc.Close()

	// Collect error from stream
	done := make(chan struct{})
	var streamError []string
	go func() {
		for line := range proc.StreamError() {
			streamError = append(streamError, line)
		}

		// Go routine end
		close(done)
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

	if len(streamError) != 2 {
		t.Errorf("streamed %d error lines, want 2", len(streamError))
	}

	// Check buffered error contains all lines
	errOutput := proc.Error()
	if !strings.Contains(errOutput, "error1") || !strings.Contains(errOutput, "error2") {
		t.Errorf("Error = %q, should contain error1 and error2", errOutput)
	}
}

func TestLinuxProcess_ExitCode(t *testing.T) {
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

			proc, _ := NewLinuxProcess(ctx, config)
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
