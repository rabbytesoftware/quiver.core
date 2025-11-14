package runtime

import (
	"context"
	"strings"
	"testing"
	"time"
)

func DarwinTestNewRuntime(t *testing.T) *DarwinRuntime {
	r := &DarwinRuntime{
		processes: make(map[string]*ProcessInfo),
	}
	return r
}

func DarwinTestExecute(t *testing.T) {
	r := DarwinTestNewRuntime(t)

	out, err := r.Execute(context.Background(), ".", []string{"/bin/echo", "hello"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.Contains(out, "hello") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func DarwinTestExecuteWithTimeout(t *testing.T) {
	r := DarwinTestNewRuntime(t)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := r.ExecuteWithTimeout(ctx, ".", []string{"/bin/sleep", "5"}, 1)
	if err == nil {
		t.Fatalf("expected timeout error, got none")
	}
}

func DarwinTestExecuteWithEnvironment(t *testing.T) {
	r := DarwinTestNewRuntime(t)

	out, err := r.ExecuteWithEnvironment(
		context.Background(),
		[]string{"/usr/bin/env"},
		map[string]string{"FOO": "BAR"},
	)
	if err != nil {
		t.Fatalf("ExecuteWithEnvironment failed: %v", err)
	}

	if !strings.Contains(out, "FOO=BAR") {
		t.Fatalf("env var missing in output: %q", out)
	}
}

func DarwinTestStartStopKill(t *testing.T) {
	r := DarwinTestNewRuntime(t)
	ctx := context.Background()

	pid, err := r.StartProcess(ctx, ".", []string{"/bin/sleep", "10"}, nil)
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	// Stop the process (context cancel)
	err = r.StopProcess(ctx, pid)
	if err != nil {
		t.Fatalf("StopProcess failed: %v", err)
	}

	// Should be dead now
	status, err := r.GetProcessStatus(ctx, pid)
	if err != nil {
		t.Fatalf("GetProcessStatus failed: %v", err)
	}

	if status != "killed" && status != "stopped" {
		t.Fatalf("unexpected status after stop: %q", status)
	}

	// Start again for kill test
	pid2, err := r.StartProcess(ctx, ".", []string{"/bin/sleep", "10"}, nil)
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	err = r.KillProcess(ctx, pid2)
	if err != nil {
		t.Fatalf("KillProcess failed: %v", err)
	}

	st, err := r.GetProcessStatus(ctx, pid2)
	if err != nil {
		t.Fatalf("GetProcessStatus failed: %v", err)
	}

	if st != "killed" {
		t.Fatalf("expected killed status, got %q", st)
	}
}

func DarwinTestProcessStatus(t *testing.T) {
	r := DarwinTestNewRuntime(t)
	ctx := context.Background()

	pid, err := r.StartProcess(ctx, ".", []string{"/bin/sleep", "1"}, nil)
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	// Immediately should be running or pending
	st, err := r.GetProcessStatus(ctx, pid)
	if err != nil {
		t.Fatalf("error getting process status: %v", err)
	}

	if st != "running" && st != "pending" {
		t.Fatalf("unexpected initial status: %q", st)
	}
}

func DarwinTestListProcesses(t *testing.T) {
	r := DarwinTestNewRuntime(t)
	ctx := context.Background()

	pid1, err := r.StartProcess(ctx, ".", []string{"/bin/sleep", "5"}, nil)
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	pid2, err := r.StartProcess(ctx, ".", []string{"/bin/sleep", "5"}, nil)
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	list, err := r.ListProcesses(ctx)
	if err != nil {
		t.Fatalf("ListProcesses failed: %v", err)
	}

	if len(list) < 2 {
		t.Fatalf("expected at least 2 processes, got %d", len(list))
	}

	found1 := false
	found2 := false
	for _, p := range list {
		if p == pid1 {
			found1 = true
		}
		if p == pid2 {
			found2 = true
		}
	}

	if !found1 || !found2 {
		t.Fatalf("not all pids returned: %v", list)
	}
}

func DarwinTestCaptureOutputAndError(t *testing.T) {
	r := DarwinTestNewRuntime(t)
	ctx := context.Background()

	pid, err := r.StartProcess(ctx, ".", []string{"/bin/sh", "-c", "echo hi; echo err >&2"}, nil)
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	// allow the process to finish
	time.Sleep(300 * time.Millisecond)

	out, err := r.CaptureOutput(ctx, pid)
	if err != nil {
		t.Fatalf("CaptureOutput failed: %v", err)
	}
	if !strings.Contains(out, "hi") {
		t.Fatalf("missing stdout: %q", out)
	}

	eo, err := r.CaptureError(ctx, pid)
	if err != nil {
		t.Fatalf("CaptureError failed: %v", err)
	}
	if !strings.Contains(eo, "err") {
		t.Fatalf("missing stderr: %q", eo)
	}
}

func DarwinTestStreamOutput(t *testing.T) {
	r := DarwinTestNewRuntime(t)
	ctx := context.Background()

	pid, err := r.StartProcess(ctx, ".", []string{"/bin/sh", "-c", "echo hi"}, nil)
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	ch, err := r.StreamOutput(ctx, pid)
	if err != nil {
		t.Fatalf("StreamOutput failed: %v", err)
	}

	select {
	case line := <-ch:
		if !strings.Contains(line, "hi") {
			t.Fatalf("unexpected streamed output: %q", line)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for streamed output")
	}
}

func DarwinTestStreamError(t *testing.T) {
	r := DarwinTestNewRuntime(t)
	ctx := context.Background()

	pid, err := r.StartProcess(ctx, ".", []string{"/bin/sh", "-c", "echo err >&2"}, nil)
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	ch, err := r.StreamError(ctx, pid)
	if err != nil {
		t.Fatalf("StreamError failed: %v", err)
	}

	select {
	case line := <-ch:
		if !strings.Contains(line, "err") {
			t.Fatalf("unexpected streamed error: %q", line)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for streamed error")
	}
}

func DarwinTestCleanup(t *testing.T) {
	r := DarwinTestNewRuntime(t)
	ctx := context.Background()

	pid, err := r.StartProcess(ctx, ".", []string{"/bin/sleep", "1"}, nil)
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	err = r.CleanupProcess(ctx, pid)
	if err != nil {
		t.Fatalf("CleanupProcess failed: %v", err)
	}

	list, _ := r.ListProcesses(ctx)
	for _, p := range list {
		if p == pid {
			t.Fatalf("process still present after cleanup")
		}
	}
}

func DarwinTestCleanupAll(t *testing.T) {
	r := DarwinTestNewRuntime(t)
	ctx := context.Background()

	_, _ = r.StartProcess(ctx, ".", []string{"/bin/sleep", "1"}, nil)
	_, _ = r.StartProcess(ctx, ".", []string{"/bin/sleep", "1"}, nil)

	time.Sleep(200 * time.Millisecond)

	err := r.CleanupAllProcesses(ctx)
	if err != nil {
		t.Fatalf("CleanupAllProcesses failed: %v", err)
	}

	list, _ := r.ListProcesses(ctx)
	if len(list) != 0 {
		t.Fatalf("expected no processes after cleanup all, got %v", list)
	}
}

func DarwinTestShutdown(t *testing.T) {
	r := DarwinTestNewRuntime(t)
	ctx := context.Background()

	_, _ = r.StartProcess(ctx, ".", []string{"/bin/sleep", "5"}, nil)

	err := r.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	list, _ := r.ListProcesses(ctx)
	if len(list) != 0 {
		t.Fatalf("expected zero processes after shutdown, got %v", list)
	}
}
