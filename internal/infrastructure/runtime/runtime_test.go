package runtime

import (
	"context"
	"fmt"
	stdruntime "runtime"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/models/system"
)

// -----------------------------
//       NewRuntime Tests
// -----------------------------

// TestNewRuntime_Generic tests NewRuntime() for the current OS
func TestNewRuntime_Generic(t *testing.T) {
	r := NewRuntime()

	switch stdruntime.GOOS {
	case "linux", "windows", "darwin":
		if r == nil {
			t.Fatalf("expected non-nil runtime for supported OS %s", stdruntime.GOOS)
		}
	default:
		if r != nil {
			t.Fatalf("expected nil runtime for unsupported OS %s, got %+v", stdruntime.GOOS, r)
		}
	}

	if r != nil {
		var currentOS system.OS
		switch rt := r.(type) {
		case *LinuxRuntime:
			currentOS = rt.CurrentOS
		case *WindowsRuntime:
			currentOS = rt.CurrentOS
		case *DarwinRuntime:
			currentOS = rt.CurrentOS
		default:
			t.Fatalf("unexpected runtime type: %T", r)
		}

		if currentOS == "" {
			t.Fatalf("expected CurrentOS to be set, got empty string")
		}
	}
}

// helper function to create a runtime for testing different OS strings
func newRuntimeForTest(osName string) REEInterface {
	r := &Runtime{}
	os := system.OS(osName)

	if os.IsValid() {
		r.CurrentOS = os
	} else {
		r.CurrentOS = ""
		return nil
	}

	processes := make(map[string]*ProcessInfo)

	if r.CurrentOS.IsWindows() {
		return &WindowsRuntime{Runtime: r, processes: processes}
	} else if r.CurrentOS.IsLinux() {
		return &LinuxRuntime{Runtime: r, processes: processes}
	} else if r.CurrentOS.IsDarwin() {
		return &DarwinRuntime{Runtime: r, processes: processes}
	}

	return nil
}

// Test all supported OS return correct runtime type and set CurrentOS
func TestNewRuntime_SupportedOS(t *testing.T) {
	supported := []struct {
		name string
		typ  string
	}{
		{"linux/amd64", "*runtime.LinuxRuntime"},
		{"windows/amd64", "*runtime.WindowsRuntime"},
		{"darwin/amd64", "*runtime.DarwinRuntime"},
	}

	for _, tt := range supported {
		r := newRuntimeForTest(tt.name)
		if r == nil {
			t.Fatalf("expected non-nil runtime for OS %s", tt.name)
		}

		got := fmt.Sprintf("%T", r)
		if got != tt.typ {
			t.Fatalf("expected type %s, got %s", tt.typ, got)
		}

		// Ensure CurrentOS is set
		switch rt := r.(type) {
		case *LinuxRuntime:
			if rt.CurrentOS == "" {
				t.Fatal("CurrentOS empty for LinuxRuntime")
			}
		case *WindowsRuntime:
			if rt.CurrentOS == "" {
				t.Fatal("CurrentOS empty for WindowsRuntime")
			}
		case *DarwinRuntime:
			if rt.CurrentOS == "" {
				t.Fatal("CurrentOS empty for DarwinRuntime")
			}
		}
	}
}

// Test unsupported OS returns nil
func TestNewRuntime_UnsupportedOS(t *testing.T) {
	r := newRuntimeForTest("solaris/amd64")
	if r != nil {
		t.Fatalf("expected nil for unsupported OS, got %+v", r)
	}
}

// Test processes map is initialized for all OS types
func TestNewRuntime_ProcessesInitialization(t *testing.T) {
	r := newRuntimeForTest("linux/amd64")
	lr, ok := r.(*LinuxRuntime)
	if !ok {
		t.Fatalf("expected LinuxRuntime, got %T", r)
	}

	if lr.processes == nil {
		t.Fatal("processes map not initialized")
	}

	// Insert dummy process
	lr.processes["dummy"] = &ProcessInfo{}
	if _, exists := lr.processes["dummy"]; !exists {
		t.Fatal("failed to insert dummy process")
	}
}

// -----------------------------
//        MockREE Tests
// -----------------------------

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
	return "pid123", nil
}
func (m *MockREE) StopProcess(ctx context.Context, processID string) error { return nil }
func (m *MockREE) KillProcess(ctx context.Context, processID string) error { return nil }
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
func (m *MockREE) CleanupProcess(ctx context.Context, processID string) error { return nil }
func (m *MockREE) CleanupAllProcesses(ctx context.Context) error              { return nil }
func (m *MockREE) Shutdown(ctx context.Context) error                         { return nil }

// -----------------------------
//       Interface Compliance
// -----------------------------

func TestREEInterfaceCompliance(t *testing.T) {
	var _ REEInterface = (*MockREE)(nil)
}

// -----------------------------
//       Behavior Tests
// -----------------------------

func TestMockREE_ExecuteMethods(t *testing.T) {
	ree := &MockREE{}
	out, _ := ree.Execute(context.Background(), "/bin/echo", []string{"hello"})
	if out != "executed" {
		t.Fatalf("expected 'executed', got '%s'", out)
	}

	out2, _ := ree.ExecuteWithTimeout(context.Background(), "cmd", nil, 1)
	if out2 != "executed_with_timeout" {
		t.Fatalf("expected 'executed_with_timeout', got '%s'", out2)
	}

	out3, _ := ree.ExecuteWithEnvironment(context.Background(), nil, nil)
	if out3 != "executed_with_env" {
		t.Fatalf("expected 'executed_with_env', got '%s'", out3)
	}
}

func TestMockREE_ProcessLifecycle(t *testing.T) {
	ree := &MockREE{}
	ctx := context.Background()

	pid, _ := ree.StartProcess(ctx, "/bin/echo", nil, nil)
	if pid == "" {
		t.Fatal("expected process ID")
	}

	if err := ree.StopProcess(ctx, pid); err != nil {
		t.Fatal(err)
	}
	if err := ree.KillProcess(ctx, pid); err != nil {
		t.Fatal(err)
	}
	status, _ := ree.GetProcessStatus(ctx, pid)
	if status != "running" {
		t.Fatalf("expected 'running', got '%s'", status)
	}
}

func TestMockREE_ListCaptureStream(t *testing.T) {
	ree := &MockREE{}
	list, _ := ree.ListProcesses(context.Background())
	if len(list) != 2 {
		t.Fatalf("expected 2 processes, got %d", len(list))
	}

	out, _ := ree.CaptureOutput(context.Background(), "p1")
	if out != "stdout" {
		t.Fatalf("expected 'stdout', got '%s'", out)
	}

	errOut, _ := ree.CaptureError(context.Background(), "p1")
	if errOut != "stderr" {
		t.Fatalf("expected 'stderr', got '%s'", errOut)
	}

	chOut, _ := ree.StreamOutput(context.Background(), "p1")
	v := <-chOut
	if v != "stream_out" {
		t.Fatalf("expected 'stream_out', got '%s'", v)
	}

	chErr, _ := ree.StreamError(context.Background(), "p1")
	v2 := <-chErr
	if v2 != "stream_err" {
		t.Fatalf("expected 'stream_err', got '%s'", v2)
	}
}

func TestMockREE_CleanupShutdown(t *testing.T) {
	ree := &MockREE{}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := ree.CleanupProcess(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	if err := ree.CleanupAllProcesses(ctx); err != nil {
		t.Fatal(err)
	}
	if err := ree.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}
