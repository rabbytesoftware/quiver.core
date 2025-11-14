package runtime

import (
	"context"
	"testing"
	"time"
)

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
