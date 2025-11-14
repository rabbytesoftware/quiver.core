package runtime

import (
	"context"
	"testing"
	"time"
)

// MockREE implements REEInterface for testing.
// You can extend this mock as needed for your actual implementation tests.
type MockREE struct{}

func (m *MockREE) Execute(ctx context.Context, path string, args []string) (string, error) {
	return "executed", nil
}

func (m *MockREE) ExecuteWithTimeout(ctx context.Context, path string, args []string, timeoutSeconds int) (string, error) {
	return "executed_with_timeout", nil
}

func (m *MockREE) ExecuteWithEnvironment(ctx context.Context, command []string, env map[string]string) (string, error) {
	return "executed_with_env", nil
}

func (m *MockREE) StartProcess(ctx context.Context, path string, command []string, args []string) (string, error) {
	return "process123", nil
}

func (m *MockREE) StopProcess(ctx context.Context, processID string) error {
	return nil
}

func (m *MockREE) KillProcess(ctx context.Context, processID string) error {
	return nil
}

func (m *MockREE) GetProcessStatus(ctx context.Context, processID string) (string, error) {
	return "running", nil
}

func (m *MockREE) ListProcesses(ctx context.Context) ([]string, error) {
	return []string{"p1", "p2"}, nil
}

func (m *MockREE) CaptureOutput(ctx context.Context, processID string) (string, error) {
	return "stdout", nil
}

func (m *MockREE) CaptureError(ctx context.Context, processID string) (string, error) {
	return "stderr", nil
}

func (m *MockREE) StreamOutput(ctx context.Context, processID string) (<-chan string, error) {
	ch := make(chan string, 1)
	ch <- "stream_out"
	close(ch)
	return ch, nil
}

func (m *MockREE) StreamError(ctx context.Context, processID string) (<-chan string, error) {
	ch := make(chan string, 1)
	ch <- "stream_err"
	close(ch)
	return ch, nil
}

func (m *MockREE) CleanupProcess(ctx context.Context, processID string) error {
	return nil
}

func (m *MockREE) CleanupAllProcesses(ctx context.Context) error {
	return nil
}

func (m *MockREE) Shutdown(ctx context.Context) error {
	return nil
}

// -----------------------------------------------------
//                INTERFACE COMPLIANCE TESTS
// -----------------------------------------------------

func TestREEInterfaceCompliance(t *testing.T) {
	var _ REEInterface = (*MockREE)(nil)
}

// -----------------------------------------------------
//                   BEHAVIOR TESTS
// -----------------------------------------------------

func TestExecute(t *testing.T) {
	ree := &MockREE{}
	out, err := ree.Execute(context.Background(), "/bin/echo", []string{"hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "executed" {
		t.Fatalf("expected 'executed', got: %s", out)
	}
}

func TestExecuteWithTimeout(t *testing.T) {
	ree := &MockREE{}
	out, err := ree.ExecuteWithTimeout(context.Background(), "cmd", nil, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "executed_with_timeout" {
		t.Fatalf("expected 'executed_with_timeout', got: %s", out)
	}
}

func TestStartStopKill(t *testing.T) {
	ree := &MockREE{}
	ctx := context.Background()

	pid, err := ree.StartProcess(ctx, "/bin/echo", nil, nil)
	if err != nil {
		t.Fatalf("StartProcess error: %v", err)
	}
	if pid == "" {
		t.Fatalf("expected process ID, got empty")
	}

	if err := ree.StopProcess(ctx, pid); err != nil {
		t.Fatalf("StopProcess error: %v", err)
	}

	if err := ree.KillProcess(ctx, pid); err != nil {
		t.Fatalf("KillProcess error: %v", err)
	}
}

func TestProcessStatus(t *testing.T) {
	ree := &MockREE{}
	status, err := ree.GetProcessStatus(context.Background(), "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "running" {
		t.Fatalf("expected 'running', got: %s", status)
	}
}

func TestListProcesses(t *testing.T) {
	ree := &MockREE{}
	list, err := ree.ListProcesses(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 processes, got %d", len(list))
	}
}

func TestCaptureOutputAndError(t *testing.T) {
	ree := &MockREE{}

	out, _ := ree.CaptureOutput(context.Background(), "p1")
	if out != "stdout" {
		t.Fatalf("expected 'stdout', got %s", out)
	}

	errOut, _ := ree.CaptureError(context.Background(), "p1")
	if errOut != "stderr" {
		t.Fatalf("expected 'stderr', got %s", errOut)
	}
}

func TestStreamOutput(t *testing.T) {
	ree := &MockREE{}
	ch, err := ree.StreamOutput(context.Background(), "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v := <-ch
	if v != "stream_out" {
		t.Fatalf("expected 'stream_out', got: %s", v)
	}
}

func TestStreamError(t *testing.T) {
	ree := &MockREE{}
	ch, err := ree.StreamError(context.Background(), "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v := <-ch
	if v != "stream_err" {
		t.Fatalf("expected 'stream_err', got: %s", v)
	}
}

func TestCleanup(t *testing.T) {
	ree := &MockREE{}
	if err := ree.CleanupProcess(context.Background(), "p1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := ree.CleanupAllProcesses(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestShutdown(t *testing.T) {
	ree := &MockREE{}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := ree.Shutdown(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// package runtime

// import (
// 	"context"
// 	"fmt"
// 	"os"
// 	"strings"
// 	"testing"
// 	"time"

// 	"github.com/rabbytesoftware/quiver/internal/core/watcher"
// )

// // helper to instantiate LinuxRuntime directly (tests should reference linux runtime as concrete)
// func newLinuxRuntimeForTest() REEInterface {
// 	return &LinuxRuntime{Runtime: &Runtime{}, processes: make(map[string]*ProcessInfo)}
// }

// // Test helper process pattern. When the test binary is re-executed with
// // -test.run=TestHelperProcess and GO_WANT_HELPER_PROCESS=1 it will run this code path
// // and act as a controllable child process.
// func TestHelperProcess(t *testing.T) {
// 	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
// 		return
// 	}
// 	// find separator "--" and treat following args as our own
// 	args := os.Args
// 	for i, a := range args {
// 		if a == "--" && i+1 < len(args) {
// 			payload := args[i+1:]
// 			switch payload[0] {
// 			case "echo":
// 				fmt.Println(strings.Join(payload[1:], " "))
// 				os.Exit(0)
// 			case "sleep":
// 				// sleep seconds then exit
// 				// payload[1] expected to be seconds
// 				// ignore errors for brevity
// 				time.Sleep(2 * time.Second)
// 				fmt.Println("done")
// 				os.Exit(0)
// 			case "stream":
// 				// print a couple of lines with small pauses and then block
// 				fmt.Println("line1")
// 				time.Sleep(50 * time.Millisecond)
// 				fmt.Println("line2")
// 				// block to allow Stop/Kill tests to signal
// 				select {}
// 			case "stderr":
// 				fmt.Fprintln(os.Stderr, "error-line")
// 				os.Exit(1)
// 			default:
// 				os.Exit(0)
// 			}
// 		}
// 	}
// 	os.Exit(0)
// }

// // TestMain ensures global services used by the runtime (like watcher) are initialized
// // before any test runs. The watcher package relies on NewWatcherService being called
// // by application bootstrap; tests that exercise runtime.CaptureOutput call watcher.Info
// // and therefore need a valid watcher instance.
// func TestMain(m *testing.M) {
// 	watcher.NewWatcherService()
// 	os.Exit(m.Run())
// }

// func TestExecute_EchoHelper(t *testing.T) {
// 	rt := newLinuxRuntimeForTest()
// 	ctx := context.Background()

// 	// arrange helper invocation
// 	helper := os.Args[0]
// 	cmd := []string{helper, "-test.run=TestHelperProcess", "--", "echo", "hello-world"}

// 	// export flag so child knows it's running as helper
// 	os.Setenv("GO_WANT_HELPER_PROCESS", "1")
// 	defer os.Unsetenv("GO_WANT_HELPER_PROCESS")

// 	out, err := rt.Execute(ctx, ".", cmd)
// 	if err != nil {
// 		t.Fatalf("Execute failed: %v", err)
// 	}
// 	if !strings.Contains(out, "hello-world") {
// 		t.Fatalf("expected helper output, got: %q", out)
// 	}
// }

// func TestExecuteWithTimeout_TimesOut(t *testing.T) {
// 	rt := newLinuxRuntimeForTest()
// 	ctx := context.Background()

// 	helper := os.Args[0]
// 	cmd := []string{helper, "-test.run=TestHelperProcess", "--", "sleep"}

// 	os.Setenv("GO_WANT_HELPER_PROCESS", "1")
// 	defer os.Unsetenv("GO_WANT_HELPER_PROCESS")

// 	// use 1s timeout while helper sleeps ~2s
// 	_, err := rt.ExecuteWithTimeout(ctx, ".", cmd, 1)
// 	if err == nil {
// 		t.Fatalf("expected timeout error, got nil")
// 	}
// }

// func TestStartProcess_StreamAndStop(t *testing.T) {
// 	rt := newLinuxRuntimeForTest()
// 	ctx := context.Background()

// 	helper := os.Args[0]
// 	cmd := []string{helper, "-test.run=TestHelperProcess", "--", "stream"}

// 	os.Setenv("GO_WANT_HELPER_PROCESS", "1")
// 	defer os.Unsetenv("GO_WANT_HELPER_PROCESS")

// 	pid, err := rt.StartProcess(ctx, ".", cmd, nil)
// 	if err != nil {
// 		t.Fatalf("StartProcess failed: %v", err)
// 	}

// 	// stream output
// 	ch, err := rt.StreamOutput(ctx, pid)
// 	if err != nil {
// 		t.Fatalf("StreamOutput failed: %v", err)
// 	}

// 	// read two lines that helper emits
// 	got := []string{}
// 	timeout := time.After(2 * time.Second)
// 	for len(got) < 2 {
// 		select {
// 		case ln, ok := <-ch:
// 			if !ok {
// 				t.Fatalf("output channel closed prematurely")
// 			}
// 			got = append(got, ln)
// 		case <-timeout:
// 			t.Fatalf("timed out waiting for stream lines, got=%v", got)
// 		}
// 	}

// 	// stop process gracefully
// 	stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
// 	defer cancel()
// 	if err := rt.StopProcess(stopCtx, pid); err != nil {
// 		t.Fatalf("StopProcess failed: %v", err)
// 	}

// 	// capture output and check contents
// 	out, err := rt.CaptureOutput(ctx, pid)
// 	if err != nil {
// 		t.Fatalf("CaptureOutput failed: %v", err)
// 	}
// 	if !strings.Contains(out, "line1") || !strings.Contains(out, "line2") {
// 		t.Fatalf("captured output missing lines: %q", out)
// 	}
// }

// func TestStreamErrorAndCaptureError(t *testing.T) {
// 	rt := newLinuxRuntimeForTest()
// 	ctx := context.Background()

// 	helper := os.Args[0]
// 	cmd := []string{helper, "-test.run=TestHelperProcess", "--", "stderr"}

// 	os.Setenv("GO_WANT_HELPER_PROCESS", "1")
// 	defer os.Unsetenv("GO_WANT_HELPER_PROCESS")

// 	pid, err := rt.StartProcess(ctx, ".", cmd, nil)
// 	if err != nil {
// 		t.Fatalf("StartProcess(stderr) failed: %v", err)
// 	}

// 	// stream error
// 	ech, err := rt.StreamError(ctx, pid)
// 	if err != nil {
// 		t.Fatalf("StreamError failed: %v", err)
// 	}

// 	// read one line from stderr
// 	select {
// 	case ln := <-ech:
// 		if !strings.Contains(ln, "error-line") {
// 			t.Fatalf("expected stderr line, got %q", ln)
// 		}
// 	case <-time.After(1 * time.Second):
// 		t.Fatalf("timed out waiting for stderr line")
// 	}

// 	// allow process to exit and then capture error
// 	// helper exits with code 1
// 	time.Sleep(200 * time.Millisecond)
// 	errMsg, err := rt.CaptureError(ctx, pid)
// 	if err != nil {
// 		t.Fatalf("CaptureError failed: %v", err)
// 	}
// 	if !strings.Contains(errMsg, "Exit with code") {
// 		t.Fatalf("unexpected error message: %q", errMsg)
// 	}
// }

// func TestNotFoundBehaviors(t *testing.T) {
// 	rt := newLinuxRuntimeForTest()
// 	ctx := context.Background()

// 	if _, err := rt.StreamOutput(ctx, "no-such"); err == nil {
// 		t.Fatalf("expected error for StreamOutput on missing pid")
// 	}
// 	if _, err := rt.CaptureOutput(ctx, "no-such"); err == nil {
// 		t.Fatalf("expected error for CaptureOutput on missing pid")
// 	}
// 	if err := rt.StopProcess(ctx, "no-such"); err == nil {
// 		t.Fatalf("expected error for StopProcess on missing pid")
// 	}
// }
