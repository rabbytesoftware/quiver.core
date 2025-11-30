//go:build windows

package process

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/infrastructure/runtime/models"
)

func TestNewWindowsProcess(t *testing.T) {
	config := models.NewConfig([]string{"cmd", "/c", "echo test"})
	ctx := context.Background()

	proc, err := NewWindowsProcess(ctx, config)
	if err != nil {
		t.Fatalf("NewWindowsProcess() error = %v", err)
	}

	if proc == nil {
		t.Fatal("NewWindowsProcess() returned nil")
	}

	if proc.BaseProcess == nil {
		t.Error("BaseProcess should not be nil")
	}

	proc.Close()
}

func TestWindowsProcess_Start(t *testing.T) {
	config := models.NewConfig([]string{"cmd", "/c", "echo Hello Windows"})
	ctx := context.Background()

	proc, _ := NewWindowsProcess(ctx, config)
	defer proc.Close()

	err := proc.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if proc.Status() != models.StatusRunning && proc.Status() != models.StatusFinished {
		t.Errorf("Status after Start() = %v, want Running or Finished", proc.Status())
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = proc.Wait(waitCtx)
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}

	// Poll for output with timeout to handle race conditions
	var output string
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		output = proc.Output()
		if strings.Contains(output, "Hello Windows") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !strings.Contains(output, "Hello Windows") {
		t.Errorf("Output = %q, should contain 'Hello Windows'", output)
	}
}

func TestWindowsProcess_Start_AlreadyStarted(t *testing.T) {
	config := models.NewConfig([]string{"cmd", "/c", "timeout /t 10"})
	ctx := context.Background()

	proc, _ := NewWindowsProcess(ctx, config)
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

func TestWindowsProcess_Stop(t *testing.T) {
	config := models.NewConfig([]string{"cmd", "/c", "timeout /t 30"})
	ctx := context.Background()

	proc, _ := NewWindowsProcess(ctx, config)
	defer proc.Close()

	err := proc.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Ensure process is actually running
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if proc.Status() == models.StatusRunning {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if proc.Status() != models.StatusRunning {
		t.Fatalf("Process never reached running state, status = %v", proc.Status())
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = proc.Stop(stopCtx)
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	// Poll for status update with timeout to handle race conditions
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if proc.Status() == models.StatusFinished {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if proc.Status() != models.StatusFinished {
		t.Errorf("Status after Stop() = %v, want %v", proc.Status(), models.StatusFinished)
	}
}

func TestWindowsProcess_Stop_NotRunning(t *testing.T) {
	config := models.NewConfig([]string{"cmd", "/c", "echo test"})
	ctx := context.Background()

	proc, _ := NewWindowsProcess(ctx, config)
	defer proc.Close()

	err := proc.Stop(ctx)
	if err != models.ErrInvalidState {
		t.Errorf("Stop() on not running process error = %v, want %v", err, models.ErrInvalidState)
	}
}

func TestWindowsProcess_Kill(t *testing.T) {
	config := models.NewConfig([]string{"cmd", "/c", "timeout /t 30"})
	ctx := context.Background()

	proc, _ := NewWindowsProcess(ctx, config)
	defer proc.Close()

	err := proc.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Ensure process is actually running
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if proc.Status() == models.StatusRunning {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if proc.Status() != models.StatusRunning {
		t.Fatalf("Process never reached running state, status = %v", proc.Status())
	}

	killCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = proc.Kill(killCtx)
	if err != nil {
		t.Errorf("Kill() error = %v", err)
	}

	// Poll for status update with timeout to handle race conditions
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if proc.Status() == models.StatusFinished {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if proc.Status() != models.StatusFinished {
		t.Errorf("Status after Kill() = %v, want %v", proc.Status(), models.StatusFinished)
	}
}

func TestWindowsProcess_OutputStreaming(t *testing.T) {
	config := models.NewConfig([]string{"cmd", "/c", "echo line1 & echo line2 & echo line3"})
	ctx := context.Background()

	proc, _ := NewWindowsProcess(ctx, config)
	defer proc.Close()

	// Collect output from stream with mutex protection
	var streamOutput []string
	var mu sync.Mutex
	go func() {
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

	// Give time for streaming to complete
	time.Sleep(100 * time.Millisecond)

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

func TestWindowsProcess_ErrorStreaming(t *testing.T) {
	// Windows command that writes to stderr
	config := models.NewConfig([]string{"cmd", "/c", "echo error1 1>&2 & echo error2 1>&2"})
	ctx := context.Background()

	proc, _ := NewWindowsProcess(ctx, config)
	defer proc.Close()

	// Collect error from stream with mutex protection
	var streamError []string
	var mu sync.Mutex
	go func() {
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

	// Give time for streaming to complete
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	errCount := len(streamError)
	mu.Unlock()

	if errCount != 2 {
		t.Errorf("streamed %d error lines, want 2", errCount)
	}

	// Check buffered error contains all lines
	errOutput := proc.Error()
	if !strings.Contains(errOutput, "error1") || !strings.Contains(errOutput, "error2") {
		t.Errorf("Error = %q, should contain error1 and error2", errOutput)
	}
}

func TestWindowsProcess_ExitCode(t *testing.T) {
	tests := []struct {
		name     string
		command  []string
		wantCode int
	}{
		{
			name:     "successful command",
			command:  []string{"cmd", "/c", "exit 0"},
			wantCode: 0,
		},
		{
			name:     "failed command",
			command:  []string{"cmd", "/c", "exit 1"},
			wantCode: 1,
		},
		{
			name:     "custom exit code",
			command:  []string{"cmd", "/c", "exit 42"},
			wantCode: 42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := models.NewConfig(tt.command)
			ctx := context.Background()

			proc, _ := NewWindowsProcess(ctx, config)
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
