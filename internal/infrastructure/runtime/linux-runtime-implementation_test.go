package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func newREE() REEInterface {
	r := &LinuxRuntime{
		processes: make(map[string]*ProcessInfo),
	}
	return r
}

func LinuxTestExecute(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}

	ree := newREE()

	out, err := ree.Execute(context.Background(), "/", []string{"echo", "hello"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if out != "hello\n" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func LinuxTestExecuteWithTimeout(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}

	ree := newREE()

	ctx := context.Background()

	_, err := ree.ExecuteWithTimeout(ctx, "/", []string{"sleep", "2"}, 1)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
}

func (r *LinuxRuntime) LinuxExecuteWithEnvironment(
	ctx context.Context,
	command []string,
	env map[string]string,
) (string, error) {
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)

	cmd.Env = os.Environ()

	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return "", err
	}

	return out.String(), nil
}

func LinuxTestStartStopKill(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}

	ree := newREE()
	ctx := context.Background()

	pid, err := ree.StartProcess(ctx, "/", []string{"sleep", "10"}, nil)
	if err != nil {
		t.Fatalf("StartProcess error: %v", err)
	}

	// stop
	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	err = ree.StopProcess(stopCtx, pid)
	if err != nil {
		t.Fatalf("StopProcess failed: %v", err)
	}

	// start again for kill test
	pid, err = ree.StartProcess(ctx, "/", []string{"sleep", "10"}, nil)
	if err != nil {
		t.Fatalf("StartProcess error: %v", err)
	}

	killCtx, cancel2 := context.WithTimeout(ctx, 2*time.Second)
	defer cancel2()

	err = ree.KillProcess(killCtx, pid)
	if err != nil {
		t.Fatalf("KillProcess failed: %v", err)
	}
}

func LinuxTestStreamOutput(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}
	ree := newREE()
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
		t.Fatalf("expected >= 5 lines, got %d", count)
	}
}

func LinuxTestStreamError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}

	ree := newREE()
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
		t.Fatalf("expected to receive stderr output")
	}
}

func LinuxTestCaptureOutputAndError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}

	ree := newREE()
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

func LinuxTestCleanup(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}

	ree := newREE()
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

func LinuxTestShutdown(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only")
	}

	ree := newREE()
	ctx := context.Background()

	ree.StartProcess(ctx, "/", []string{"sleep", "5"}, nil)
	ree.StartProcess(ctx, "/", []string{"sleep", "5"}, nil)

	if err := ree.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}
