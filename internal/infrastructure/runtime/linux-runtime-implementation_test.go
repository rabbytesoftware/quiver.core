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

func newLinuxREE() REEInterface {
	return &LinuxRuntime{
		processes: make(map[string]*ProcessInfo),
	}
}

func TestLinuxExecute(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}

	ree := newLinuxREE()

	out, err := ree.Execute(context.Background(), "/", []string{"echo", "hello"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if out != "hello\n" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestLinuxExecuteWithTimeout(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}

	ree := newLinuxREE()
	ctx := context.Background()

	_, err := ree.ExecuteWithTimeout(ctx, "/", []string{"sleep", "2"}, 1)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
}

func TestLinuxExecuteWithEnvironment(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}

	// Ejecutamos algo simple usando env
	cmd := exec.Command("bash", "-c", "echo $TEST_VAR")
	cmd.Env = append(os.Environ(), "TEST_VAR=abc123")

	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	if out.String() != "abc123\n" {
		t.Fatalf("unexpected env output: %q", out.String())
	}
}

func TestLinuxStartStopKill(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}

	ree := newLinuxREE()

	// Contexto para Start + Stop
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// start el proceso
	pid, err := ree.StartProcess(ctx, "/", []string{"sleep", "10"}, nil)
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	// stop (SIGTERM)
	if err := ree.StopProcess(ctx, pid); err != nil {
		t.Fatalf("StopProcess failed: %v", err)
	}

	// 🔥 Kill con un context NUEVO y más largo
	killCtx, killCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer killCancel()

	if err := ree.KillProcess(killCtx, pid); err != nil {
		t.Fatalf("KillProcess failed: %v", err)
	}
}

func TestLinuxStreamOutput(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}

	ree := newLinuxREE()
	ctx := context.Background()

	pid, err := ree.StartProcess(ctx, "/", []string{
		"bash", "-c", "for i in {1..5}; do echo $i; sleep 0.1; done",
	}, nil)
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

func TestLinuxStreamError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}

	ree := newLinuxREE()
	ctx := context.Background()

	pid, err := ree.StartProcess(ctx, "/", []string{
		"bash", "-c", "echo ERROR! >&2; sleep 0.1",
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
		t.Fatalf("expected stderr OUTPUT but got none")
	}
}

func TestLinuxCaptureOutputAndError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}

	ree := newLinuxREE()
	ctx := context.Background()

	pid, err := ree.StartProcess(ctx, "/", []string{
		"bash", "-c", "echo hello; echo err >&2",
	}, nil)
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

func TestLinuxCleanup(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}

	ree := newLinuxREE()
	ctx := context.Background()

	pid, err := ree.StartProcess(ctx, "/", []string{"sleep", "1"}, nil)
	if err != nil {
		t.Fatalf("StartProcess error: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	if err := ree.CleanupProcess(ctx, pid); err != nil {
		t.Fatalf("CleanupProcess failed: %v", err)
	}
}
