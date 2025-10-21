package runtime

import (
	"context"
	"fmt"
	"runtime"

	"github.com/rabbytesoftware/quiver/internal/models/system"
)

type Runtime struct {
	REEImplementation REEInterface
}

var CurrentOS system.OS

func NewRuntime() REEInterface {
	// just for testing purpose
	ctx := context.Background()
  r := &Runtime{}

	// test functions
  output, err := r.Execute(ctx, "~/Docs",[]string{"test"})
  if err != nil {
    fmt.Println("Error:", err)
  }

  fmt.Println("Result:", output)

	// set OS global variable
	os := system.OS(runtime.GOOS + "/" + runtime.GOARCH)

	// check if OS is supported
	if os.IsValid() {
		CurrentOS = os
	} else {
		fmt.Println("Unsupported operative system:", os)
		CurrentOS = ""
	}

	// handle each OS
	if CurrentOS.IsWindows() {
		return &WindowsRuntime{Runtime: &Runtime{}}
	} else if CurrentOS.IsLinux() {
		return &LinuxRuntime{Runtime: &Runtime{}}
	} else if CurrentOS.IsDarwin() {
		return &DarwinRuntime{Runtime: &Runtime{}}
	}
	
	return &Runtime{}
}

func (r *Runtime) Execute(
	ctx context.Context,
	path string,
	args []string,
) (string, error) {
	// this runs synchronously in the background

	return "", nil
}

func (r *Runtime) ExecuteWithTimeout(
	ctx context.Context,
	path string,
	args []string,
	timeoutSeconds int,
) (string, error) {
	// max execution time = timeout

	// this requires waiting asynchronous 

	return "", nil
}

func (r *Runtime) ExecuteWithEnvironment(
	ctx context.Context,
	command []string,
	env map[string]string,
) (string, error) {
	return "", nil
}

func (r *Runtime) StartProcess(
	ctx context.Context,
	command []string,
) (string, error) {
	return "", nil
}

func (r *Runtime) StopProcess(
	ctx context.Context,
	processID string,
) error {
	return nil
}

func (r *Runtime) KillProcess(
	ctx context.Context,
	processID string,
) error {
	return nil
}

func (r *Runtime) GetProcessStatus(
	ctx context.Context,
	processID string,
) (string, error) {
	return "", nil
}

func (r *Runtime) ListProcesses(
	ctx context.Context,
) ([]string, error) {
	return nil, nil
}

func (r *Runtime) CaptureOutput(
	ctx context.Context,
	processID string,
) (string, error) {
	return "", nil
}

func (r *Runtime) CaptureError(
	ctx context.Context,
	processID string,
) (string, error) {
	return "", nil
}

func (r *Runtime) StreamOutput(
	ctx context.Context,
	processID string,
) (<-chan string, error) {
	return nil, nil
}

func (r *Runtime) StreamError(
	ctx context.Context,
	processID string,
) (<-chan string, error) {
	return nil, nil
}

func (r *Runtime) GetPoolSize(
	ctx context.Context,
) (int, error) {
	return 0, nil
}

func (r *Runtime) SetPoolSize(
	ctx context.Context,
	size int,
) error {
	return nil
}

func (r *Runtime) GetAvailableExecutors(
	ctx context.Context,
) (int, error) {
	return 0, nil
}

func (r *Runtime) GetActiveExecutors(
	ctx context.Context,
) (int, error) {
	return 0, nil
}

func (r *Runtime) CleanupProcess(
	ctx context.Context,
	processID string,
) error {
	return nil
}

func (r *Runtime) CleanupAllProcesses(
	ctx context.Context,
) error {
	return nil
}

func (r *Runtime) Shutdown(
	ctx context.Context,
) error {
	return nil
}
