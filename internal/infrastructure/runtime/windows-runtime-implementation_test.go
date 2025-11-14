package runtime

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func newWindowsREE() REEInterface {
	return &WindowsRuntime{
		processes: make(map[string]*ProcessInfo),
	}
}

func TestWindowsExecute(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}

	ree := newWindowsREE()

	out, err := ree.Execute(context.Background(), "C:\\", []string{"cmd", "/C", "echo hello"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if out != "hello\r\n" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestWindowsExecuteWithTimeout(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}

	ree := newWindowsREE()
	ctx := context.Background()

	// Timeout debe cortar `timeout /T 2`
	_, err := ree.ExecuteWithTimeout(ctx, "C:\\", []string{"cmd", "/C", "timeout /T 2"}, 1)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
}

func TestWindowsExecuteWithEnvironment(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}

	// echo %VAR% en Windows
	cmd := exec.Command("cmd", "/C", "echo %TEST_VAR%")
	cmd.Env = append(os.Environ(), "TEST_VAR=abc123")

	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	if out.String() != "abc123\r\n" {
		t.Fatalf("unexpected env output: %q", out.String())
	}
}

func TestWindowsStartStopKill(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}

	ree := newWindowsREE()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Start: timeout /T 10
	pid, err := ree.StartProcess(ctx, "C:\\", []string{"cmd", "/C", "timeout /T 10"}, nil)
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	// Stop = SIGTERM equivalente → taskkill /PID pid
	if err := ree.StopProcess(ctx, pid); err != nil {
		t.Fatalf("StopProcess failed: %v", err)
	}

	// Kill con ctx independiente
	killCtx, killCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer killCancel()

	if err := ree.KillProcess(killCtx, pid); err != nil {
		t.Fatalf("KillProcess failed: %v", err)
	}
}

func TestWindowsStreamOutput(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}

	ree := newWindowsREE()
	ctx := context.Background()

	// echo 1 & echo 2 & echo 3 ...
	cmd := "echo 1 & echo 2 & echo 3 & echo 4 & echo 5"
	pid, err := ree.StartProcess(ctx, "C:\\", []string{"cmd", "/C", cmd}, nil)
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	outChan, err := ree.StreamOutput(ctx, pid)
	if err != nil {
		t.Fatalf("StreamOutput error: %v", err)
	}

	count := 0
	timeout := time.After(2 * time.Second)

loop:
	for {
		select {
		case _, ok := <-outChan:
			if !ok {
				break loop
			}
			count++
		case <-timeout:
			t.Fatalf("timeout waiting for stream output")
		}
	}

	if count < 5 {
		t.Fatalf("expected >= 5 streamed lines, got %d", count)
	}
}

func TestWindowsStreamError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}

	ree := newWindowsREE()
	ctx := context.Background()

	// Genera stderr en Windows con: cmd /C (1>&2 echo ERROR!)
	pid, err := ree.StartProcess(ctx, "C:\\", []string{
		"cmd", "/C", "(1>&2 echo ERROR!)",
	}, nil)
	if err != nil {
		t.Fatalf("StartProcess error: %v", err)
	}

	errChan, err := ree.StreamError(ctx, pid)
	if err != nil {
		t.Fatalf("StreamError error: %v", err)
	}

	timeout := time.After(2 * time.Second)
	got := false

loop:
	for {
		select {
		case line, ok := <-errChan:
			if !ok {
				break loop
			}
			if line == "ERROR!" {
				got = true
			}
		case <-timeout:
			break loop
		}
	}

	if !got {
		t.Fatalf("expected stderr output but got none")
	}
}

func TestWindowsCaptureOutputAndError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}

	ree := newWindowsREE()
	ctx := context.Background()

	// stdout + stderr
	cmd := "echo hello & (1>&2 echo err)"
	pid, err := ree.StartProcess(ctx, "C:\\", []string{"cmd", "/C", cmd}, nil)
	if err != nil {
		t.Fatalf("StartProcess error: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	out, _ := ree.CaptureOutput(ctx, pid)
	errOut, _ := ree.CaptureError(ctx, pid)

	if out == "" {
		t.Fatalf("expected stdout, got empty")
	}
	if errOut == "" {
		t.Fatalf("expected stderr, got empty")
	}
}

func TestWindowsCleanup(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}

	ree := newWindowsREE()
	ctx := context.Background()

	pid, err := ree.StartProcess(ctx, "C:\\", []string{"cmd", "/C", "timeout /T 1"}, nil)
	if err != nil {
		t.Fatalf("StartProcess error: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if err := ree.CleanupProcess(ctx, pid); err != nil {
		t.Fatalf("CleanupProcess failed: %v", err)
	}
}
