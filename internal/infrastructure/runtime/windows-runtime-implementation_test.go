package runtime

import (
	"context"
	"strings"
	"testing"
	"time"
)

func WindowsTestNewRuntime(t *testing.T) *WindowsRuntime {
	return &WindowsRuntime{
		processes: make(map[string]*ProcessInfo),
	}
}

func WindowsTestExecute(t *testing.T) {
	r := WindowsTestNewRuntime(t)

	out, err := r.Execute(context.Background(), ".", []string{"echo hello"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.Contains(out, "hello") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func WindowsTestExecuteWithTimeout(t *testing.T) {
	r := WindowsTestNewRuntime(t)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// timeout immediate
	_, err := r.ExecuteWithTimeout(ctx, ".", []string{"timeout /T 5"}, 1)
	if err == nil {
		t.Fatalf("expected timeout error, got none")
	}
}

func WindowsTestExecuteWithEnvironment(t *testing.T) {
	r := WindowsTestNewRuntime(t)

	out, err := r.ExecuteWithEnvironment(
		context.Background(),
		[]string{"cmd", "/C", "set"},
		map[string]string{"FOO": "BAR"},
	)
	if err != nil {
		t.Fatalf("ExecuteWithEnvironment failed: %v", err)
	}

	if !strings.Contains(out, "FOO=BAR") {
		t.Fatalf("env variable not found in output: %q", out)
	}
}

func WindowsTestStartStopKill(t *testing.T) {
	r := WindowsTestNewRuntime(t)
	ctx := context.Background()

	pid, err := r.StartProcess(ctx, ".", []string{"cmd", "/C", "timeout", "/T", "10"}, nil)
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	// stop = kill
	if err := r.StopProcess(ctx, pid); err != nil {
		t.Fatalf("StopProcess failed: %v", err)
	}

	status, err := r.GetProcessStatus(ctx, pid)
	if err != nil {
		t.Fatalf("GetProcessStatus failed: %v", err)
	}

	if status != "stopping" && status != "finished" {
		t.Fatalf("unexpected status after stop: %q", status)
	}

	// now kill another
	pid2, err := r.StartProcess(ctx, ".", []string{"cmd", "/C", "timeout", "/T", "10"}, nil)
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	if err := r.KillProcess(ctx, pid2); err != nil {
		t.Fatalf("KillProcess failed: %v", err)
	}
}

func WindowsTestProcessStatus(t *testing.T) {
	r := WindowsTestNewRuntime(t)
	ctx := context.Background()

	pid, err := r.StartProcess(ctx, ".", []string{"cmd", "/C", "timeout", "/T", "2"}, nil)
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	st, err := r.GetProcessStatus(ctx, pid)
	if err != nil {
		t.Fatalf("GetProcessStatus failed: %v", err)
	}

	if st != "running" {
		t.Fatalf("expected running, got %q", st)
	}
}

func WindowsTestListProcesses(t *testing.T) {
	r := WindowsTestNewRuntime(t)
	ctx := context.Background()

	_, err := r.StartProcess(ctx, ".", []string{"cmd", "/C", "timeout /T 5"}, nil)
	if err != nil {
		t.Fatalf("StartProcess 1 failed: %v", err)
	}

	_, err = r.StartProcess(ctx, ".", []string{"cmd", "/C", "timeout /T 5"}, nil)
	if err != nil {
		t.Fatalf("StartProcess 2 failed: %v", err)
	}

	list, err := r.ListProcesses(ctx)
	if err != nil {
		t.Fatalf("ListProcesses failed: %v", err)
	}

	if len(list) < 2 {
		t.Fatalf("expected >=2 processes, got %d", len(list))
	}
}

func WindowsTestCaptureOutputAndError(t *testing.T) {
	r := WindowsTestNewRuntime(t)
	ctx := context.Background()

	pid, err := r.StartProcess(ctx, ".", []string{
		"cmd", "/C", "echo hi & echo err 1>&2",
	}, nil)
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	// wait
	time.Sleep(500 * time.Millisecond)

	out, err := r.CaptureOutput(ctx, pid)
	if err != nil {
		t.Fatalf("CaptureOutput failed: %v", err)
	}
	if !strings.Contains(out, "hi") {
		t.Fatalf("stdout missing: %q", out)
	}

	er, err := r.CaptureError(ctx, pid)
	if err != nil {
		t.Fatalf("CaptureError failed: %v", err)
	}
	if !strings.Contains(er, "err") {
		t.Fatalf("stderr missing: %q", er)
	}
}

func WindowsTestStreamOutput(t *testing.T) {
	r := WindowsTestNewRuntime(t)
	ctx := context.Background()

	pid, err := r.StartProcess(ctx, ".", []string{"cmd", "/C", "echo hi"}, nil)
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
			t.Fatalf("unexpected output: %q", line)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for stream")
	}
}

func WindowsTestStreamError(t *testing.T) {
	r := WindowsTestNewRuntime(t)
	ctx := context.Background()

	pid, err := r.StartProcess(ctx, ".", []string{
		"cmd", "/C", "echo err 1>&2",
	}, nil)
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
			t.Fatalf("unexpected error output: %q", line)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for error stream")
	}
}

func WindowsTestCleanup(t *testing.T) {
	r := WindowsTestNewRuntime(t)
	ctx := context.Background()

	pid, err := r.StartProcess(ctx, ".", []string{"cmd", "/C", "timeout /T 1"}, nil)
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	if err := r.CleanupProcess(ctx, pid); err != nil {
		t.Fatalf("CleanupProcess failed: %v", err)
	}

	_, err = r.GetProcessStatus(ctx, pid)
	if err == nil {
		t.Fatalf("expected process to be removed from map")
	}
}

func WindowsTestCleanupAll(t *testing.T) {
	r := WindowsTestNewRuntime(t)
	ctx := context.Background()

	_, _ = r.StartProcess(ctx, ".", []string{"cmd", "/C", "timeout /T 1"}, nil)
	_, _ = r.StartProcess(ctx, ".", []string{"cmd", "/C", "timeout /T 1"}, nil)

	time.Sleep(300 * time.Millisecond)

	if err := r.CleanupAllProcesses(ctx); err != nil {
		t.Fatalf("CleanupAllProcesses failed: %v", err)
	}

	if len(r.processes) != 0 {
		t.Fatalf("expected 0 processes, got %d", len(r.processes))
	}
}

func WindowsTestShutdown(t *testing.T) {
	r := WindowsTestNewRuntime(t)
	ctx := context.Background()

	_, _ = r.StartProcess(ctx, ".", []string{"cmd", "/C", "timeout /T 5"}, nil)

	if err := r.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	if len(r.processes) != 0 {
		t.Fatalf("expected 0 processes after shutdown")
	}
}
