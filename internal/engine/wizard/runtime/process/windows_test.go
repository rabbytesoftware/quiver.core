//go:build windows

package process

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/models"
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ticker := time.NewTicker(1 * time.Millisecond)
	defer ticker.Stop()

	PollStart:
	for {
		if proc.Status() == models.StatusRunning {
			break
		}
		select {
		case <-ctx.Done():
			break PollStart
		case <-ticker.C:
		}
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
	ctx, cancel = context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	ticker = time.NewTicker(1 * time.Millisecond)
	defer ticker.Stop()

	PollStop:
	for {
		if proc.Status() == models.StatusFinished {
			break
		}
		select {
		case <-ctx.Done():
			break PollStop
		case <-ticker.C:
		}
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ticker := time.NewTicker(1 * time.Millisecond)
	defer ticker.Stop()

	PollKillStart:
	for {
		if proc.Status() == models.StatusRunning {
			break
		}
		select {
		case <-ctx.Done():
			break PollKillStart
		case <-ticker.C:
		}
	}

	if proc.Status() != models.StatusRunning {
		t.Fatalf("Process never reached running state, status = %v", proc.Status())
	}

	killCtx, killCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer killCancel()

	err = proc.Kill(killCtx)
	if err != nil {
		t.Errorf("Kill() error = %v", err)
	}

	// Poll for status update with timeout to handle race conditions
	pollCtx, pollCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer pollCancel()
	pollTicker := time.NewTicker(1 * time.Millisecond)
	defer pollTicker.Stop()

	PollKillStatus:
	for {
		if proc.Status() == models.StatusFinished {
			break
		}
		select {
		case <-pollCtx.Done():
			break PollKillStatus
		case <-pollTicker.C:
		}
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

	// Poll for streaming to complete with timeout
	streamCtx, streamCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer streamCancel()
	streamTicker := time.NewTicker(1 * time.Millisecond)
	defer streamTicker.Stop()

	PollStream:
	for {
		mu.Lock()
		count := len(streamOutput)
		mu.Unlock()
		if count == 3 {
			break
		}
		select {
		case <-streamCtx.Done():
			break PollStream
		case <-streamTicker.C:
		}
	}

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

	// Poll for streaming to complete with timeout
	errCtx, errCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer errCancel()
	errTicker := time.NewTicker(1 * time.Millisecond)
	defer errTicker.Stop()

	PollErr:
	for {
		mu.Lock()
		count := len(streamError)
		mu.Unlock()
		if count == 2 {
			break
		}
		select {
		case <-errCtx.Done():
			break PollErr
		case <-errTicker.C:
		}
	}

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
