package runtime

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
)

type DarwinRuntime struct {
	*Runtime
	processes   map[string]*ProcessInfo
	processLock sync.RWMutex
}

func (r *DarwinRuntime) Execute(ctx context.Context, path string, args []string) (string, error) {
	// execute command context
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = path

	// wait until execution ends
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("darwin exec error: %w", err)
	}

	return string(out), nil
}

func (r *DarwinRuntime) StartProcess(
	ctx context.Context,
	path string,
	command []string,
	args []string,
) (string, error) {
	return "", nil
}

func (r *DarwinRuntime) StopProcess(
	ctx context.Context,
	processID string,
) error {
	return nil
}

func (r *DarwinRuntime) KillProcess(
	ctx context.Context,
	processID string,
) error {
	return nil
}

func (r *DarwinRuntime) GetProcessStatus(
	ctx context.Context,
	processID string,
) (string, error) {
	return "", nil
}

func (r *DarwinRuntime) ListProcesses(
	ctx context.Context,
) ([]string, error) {
	return nil, nil
}

func (r *DarwinRuntime) CaptureOutput(
	ctx context.Context,
	processID string,
) (string, error) {
	return "", nil
}

func (r *DarwinRuntime) CaptureError(
	ctx context.Context,
	processID string,
) (string, error) {
	return "", nil
}

func (r *DarwinRuntime) StreamOutput(
	ctx context.Context,
	processID string,
) (<-chan string, error) {
	return nil, nil
}

func (r *DarwinRuntime) StreamError(
	ctx context.Context,
	processID string,
) (<-chan string, error) {
	return nil, nil
}

func (r *DarwinRuntime) GetPoolSize(
	ctx context.Context,
) (int, error) {
	return 0, nil
}

func (r *DarwinRuntime) SetPoolSize(
	ctx context.Context,
	size int,
) error {
	return nil
}

func (r *DarwinRuntime) GetAvailableExecutors(
	ctx context.Context,
) (int, error) {
	return 0, nil
}

func (r *DarwinRuntime) GetActiveExecutors(
	ctx context.Context,
) (int, error) {
	return 0, nil
}

func (r *DarwinRuntime) CleanupProcess(
	ctx context.Context,
	processID string,
) error {
	return nil
}

func (r *DarwinRuntime) CleanupAllProcesses(
	ctx context.Context,
) error {
	return nil
}

func (r *DarwinRuntime) Shutdown(
	ctx context.Context,
) error {
	return nil
}
