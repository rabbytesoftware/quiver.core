package runtime

import (
	"context"
	"fmt"
	"os/exec"
)

type WindowsRuntime struct {
	*Runtime
}

func (r *WindowsRuntime) Execute(ctx context.Context, path string, args []string) (string, error) {
	// execute command context
	cmd := exec.CommandContext(ctx, "cmd", append([]string{"/C"}, args...)...)
	cmd.Dir = path

	// wait until execution ends
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("windows exec error: %w", err)
	}

	return string(out), nil
}

func (r *WindowsRuntime) StartProcess(
	ctx context.Context,
	command []string,
) (string, error) {
	return "", nil
}

func (r *WindowsRuntime) StopProcess(
	ctx context.Context,
	processID string,
) error {
	return nil
}

func (r *WindowsRuntime) KillProcess(
	ctx context.Context,
	processID string,
) error {
	return nil
}

func (r *WindowsRuntime) GetProcessStatus(
	ctx context.Context,
	processID string,
) (string, error) {
	return "", nil
}

func (r *WindowsRuntime) ListProcesses(
	ctx context.Context,
) ([]string, error) {
	return nil, nil
}

func (r *WindowsRuntime) CaptureOutput(
	ctx context.Context,
	processID string,
) (string, error) {
	return "", nil
}

func (r *WindowsRuntime) CaptureError(
	ctx context.Context,
	processID string,
) (string, error) {
	return "", nil
}

func (r *WindowsRuntime) StreamOutput(
	ctx context.Context,
	processID string,
) (<-chan string, error) {
	return nil, nil
}

func (r *WindowsRuntime) StreamError(
	ctx context.Context,
	processID string,
) (<-chan string, error) {
	return nil, nil
}

func (r *WindowsRuntime) GetPoolSize(
	ctx context.Context,
) (int, error) {
	return 0, nil
}

func (r *WindowsRuntime) SetPoolSize(
	ctx context.Context,
	size int,
) error {
	return nil
}

func (r *WindowsRuntime) GetAvailableExecutors(
	ctx context.Context,
) (int, error) {
	return 0, nil
}

func (r *WindowsRuntime) GetActiveExecutors(
	ctx context.Context,
) (int, error) {
	return 0, nil
}

func (r *WindowsRuntime) CleanupProcess(
	ctx context.Context,
	processID string,
) error {
	return nil
}

func (r *WindowsRuntime) CleanupAllProcesses(
	ctx context.Context,
) error {
	return nil
}

func (r *WindowsRuntime) Shutdown(
	ctx context.Context,
) error {
	return nil
}
