package runtime

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// helper to instantiate LinuxRuntime directly (tests should reference linux runtime as concrete)
func newLinuxRuntimeForTest() REEInterface {
	return &LinuxRuntime{Runtime: &Runtime{}, processes: make(map[string]*ProcessInfo)}
}

// Test helper process pattern. When the test binary is re-executed with
// -test.run=TestHelperProcess and GO_WANT_HELPER_PROCESS=1 it will run this code path
// and act as a controllable child process.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	// find separator "--" and treat following args as our own
	args := os.Args
	for i, a := range args {
		if a == "--" && i+1 < len(args) {
			payload := args[i+1:]
			switch payload[0] {
			case "echo":
				fmt.Println(strings.Join(payload[1:], " "))
				os.Exit(0)
			case "sleep":
				// sleep seconds then exit
				// payload[1] expected to be seconds
				// ignore errors for brevity
				time.Sleep(2 * time.Second)
				fmt.Println("done")
				os.Exit(0)
			case "stream":
				// print a couple of lines with small pauses and then block
				fmt.Println("line1")
				time.Sleep(50 * time.Millisecond)
				fmt.Println("line2")
				// block to allow Stop/Kill tests to signal
				select {}
			case "stderr":
				fmt.Fprintln(os.Stderr, "error-line")
				os.Exit(1)
			default:
				os.Exit(0)
			}
		}
	}
	os.Exit(0)
}

func TestExecute_EchoHelper(t *testing.T) {
	rt := newLinuxRuntimeForTest()
	ctx := context.Background()

	// arrange helper invocation
	helper := os.Args[0]
	cmd := []string{helper, "-test.run=TestHelperProcess", "--", "echo", "hello-world"}

	// export flag so child knows it's running as helper
	os.Setenv("GO_WANT_HELPER_PROCESS", "1")
	defer os.Unsetenv("GO_WANT_HELPER_PROCESS")

	out, err := rt.Execute(ctx, ".", cmd)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(out, "hello-world") {
		t.Fatalf("expected helper output, got: %q", out)
	}
}

func TestExecuteWithTimeout_TimesOut(t *testing.T) {
	rt := newLinuxRuntimeForTest()
	ctx := context.Background()

	helper := os.Args[0]
	cmd := []string{helper, "-test.run=TestHelperProcess", "--", "sleep"}

	os.Setenv("GO_WANT_HELPER_PROCESS", "1")
	defer os.Unsetenv("GO_WANT_HELPER_PROCESS")

	// use 1s timeout while helper sleeps ~2s
	_, err := rt.ExecuteWithTimeout(ctx, ".", cmd, 1)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
}

func TestStartProcess_StreamAndStop(t *testing.T) {
	rt := newLinuxRuntimeForTest()
	ctx := context.Background()

	helper := os.Args[0]
	cmd := []string{helper, "-test.run=TestHelperProcess", "--", "stream"}

	os.Setenv("GO_WANT_HELPER_PROCESS", "1")
	defer os.Unsetenv("GO_WANT_HELPER_PROCESS")

	pid, err := rt.StartProcess(ctx, ".", cmd, nil)
	if err != nil {
		t.Fatalf("StartProcess failed: %v", err)
	}

	// stream output
	ch, err := rt.StreamOutput(ctx, pid)
	if err != nil {
		t.Fatalf("StreamOutput failed: %v", err)
	}

	// read two lines that helper emits
	got := []string{}
	timeout := time.After(2 * time.Second)
	for len(got) < 2 {
		select {
		case ln, ok := <-ch:
			if !ok {
				t.Fatalf("output channel closed prematurely")
			}
			got = append(got, ln)
		case <-timeout:
			t.Fatalf("timed out waiting for stream lines, got=%v", got)
		}
	}

	// stop process gracefully
	stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rt.StopProcess(stopCtx, pid); err != nil {
		t.Fatalf("StopProcess failed: %v", err)
	}

	// capture output and check contents
	out, err := rt.CaptureOutput(ctx, pid)
	if err != nil {
		t.Fatalf("CaptureOutput failed: %v", err)
	}
	if !strings.Contains(out, "line1") || !strings.Contains(out, "line2") {
		t.Fatalf("captured output missing lines: %q", out)
	}
}

func TestStreamErrorAndCaptureError(t *testing.T) {
	rt := newLinuxRuntimeForTest()
	ctx := context.Background()

	helper := os.Args[0]
	cmd := []string{helper, "-test.run=TestHelperProcess", "--", "stderr"}

	os.Setenv("GO_WANT_HELPER_PROCESS", "1")
	defer os.Unsetenv("GO_WANT_HELPER_PROCESS")

	pid, err := rt.StartProcess(ctx, ".", cmd, nil)
	if err != nil {
		t.Fatalf("StartProcess(stderr) failed: %v", err)
	}

	// stream error
	ech, err := rt.StreamError(ctx, pid)
	if err != nil {
		t.Fatalf("StreamError failed: %v", err)
	}

	// read one line from stderr
	select {
	case ln := <-ech:
		if !strings.Contains(ln, "error-line") {
			t.Fatalf("expected stderr line, got %q", ln)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for stderr line")
	}

	// allow process to exit and then capture error
	// helper exits with code 1
	time.Sleep(200 * time.Millisecond)
	errMsg, err := rt.CaptureError(ctx, pid)
	if err != nil {
		t.Fatalf("CaptureError failed: %v", err)
	}
	if !strings.Contains(errMsg, "Exit with code") {
		t.Fatalf("unexpected error message: %q", errMsg)
	}
}

func TestNotFoundBehaviors(t *testing.T) {
	rt := newLinuxRuntimeForTest()
	ctx := context.Background()

	if _, err := rt.StreamOutput(ctx, "no-such"); err == nil {
		t.Fatalf("expected error for StreamOutput on missing pid")
	}
	if _, err := rt.CaptureOutput(ctx, "no-such"); err == nil {
		t.Fatalf("expected error for CaptureOutput on missing pid")
	}
	if err := rt.StopProcess(ctx, "no-such"); err == nil {
		t.Fatalf("expected error for StopProcess on missing pid")
	}
}

