package runtime

import (
	"context"
	"fmt"
	"os/exec"
)

type LinuxRuntime struct {
	*Runtime
}

func (r *LinuxRuntime) Execute(ctx context.Context, path string, args []string) (string, error) {
	// execute command context
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = path

	// wait until execution ends
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("linux exec error: %w", err)
	}

	return string(out), nil
}

func (r *LinuxRuntime) ExecuteWithTimeout(
	ctx context.Context,
	path string,
	args []string,
	timeoutSeconds int,
) (string, error) {
	// Tiempo máximo de ejecución = timeout

	// este require de esperar asicronico

	return "", nil
}

func (r *LinuxRuntime) ExecuteWithEnvironment(
	ctx context.Context,
	command []string,
	env map[string]string,
) (string, error) {
	return "", nil
}

func (r *LinuxRuntime) StartProcess(
	ctx context.Context,
	command []string,
) (string, error) {
	return "", nil
}

func (r *LinuxRuntime) StopProcess(
	ctx context.Context,
	processID string,
) error {
	return nil
}

func (r *LinuxRuntime) KillProcess(
	ctx context.Context,
	processID string,
) error {
	return nil
}

func (r *LinuxRuntime) GetProcessStatus(
	ctx context.Context,
	processID string,
) (string, error) {
	return "", nil
}

func (r *LinuxRuntime) ListProcesses(
	ctx context.Context,
) ([]string, error) {
	return nil, nil
}

func (r *LinuxRuntime) CaptureOutput(
	ctx context.Context,
	processID string,
) (string, error) {
	return "", nil
}

func (r *LinuxRuntime) CaptureError(
	ctx context.Context,
	processID string,
) (string, error) {
	return "", nil
}

func (r *LinuxRuntime) StreamOutput(
	ctx context.Context,
	processID string,
) (<-chan string, error) {
	return nil, nil
}

func (r *LinuxRuntime) StreamError(
	ctx context.Context,
	processID string,
) (<-chan string, error) {
	return nil, nil
}

func (r *LinuxRuntime) GetPoolSize(
	ctx context.Context,
) (int, error) {
	return 0, nil
}

func (r *LinuxRuntime) SetPoolSize(
	ctx context.Context,
	size int,
) error {
	return nil
}

func (r *LinuxRuntime) GetAvailableExecutors(
	ctx context.Context,
) (int, error) {
	return 0, nil
}

func (r *LinuxRuntime) GetActiveExecutors(
	ctx context.Context,
) (int, error) {
	return 0, nil
}

func (r *LinuxRuntime) CleanupProcess(
	ctx context.Context,
	processID string,
) error {
	return nil
}

func (r *LinuxRuntime) CleanupAllProcesses(
	ctx context.Context,
) error {
	return nil
}

func (r *LinuxRuntime) Shutdown(
	ctx context.Context,
) error {
	return nil
}
