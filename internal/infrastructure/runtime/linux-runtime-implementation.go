package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

type LinuxRuntime struct {
	*Runtime
	processes   map[string]*ProcessInfo
	processLock sync.RWMutex
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
	// create channel
	resultChan := make(chan struct {
		result string
		err    error
	})

	// execute command on a goroutine
	go func() {
		result, err := r.Execute(ctx, path, args)

		resultChan <- struct {
			result string
			err    error
		}{result, err}
	}()

	// wait until result or timeout
	select {
	// handle result
	case res := <-resultChan:

		if res.err != nil {
			return "", res.err
		}

		return res.result, nil

		// abort after timeout ends
	case <-time.After(time.Duration(timeoutSeconds)):
		return "", fmt.Errorf("execution timeout after %ds, aborting", timeoutSeconds)
	}
}

func (r *LinuxRuntime) ExecuteWithEnvironment(
	ctx context.Context,
	command []string,
	env map[string]string,
) (string, error) {
	// set current directory as working directory if not specified
	path := "."

	if len(command) == 0 {
		return "", fmt.Errorf("command cannot be empty")
	}

	// get timeout from environment or use default
	timeoutSeconds := 120 // default

	if timeoutStr, exists := env["TIMEOUT_SECONDS"]; exists {
		timeout, err := strconv.Atoi(timeoutStr)

		if err != nil {
			return "", fmt.Errorf("invalid TIMEOUT_SECONDS value: %w", err)
		}

		if timeout > 0 {
			timeoutSeconds = timeout
		}
	}

	// convert environment map to slice
	envSlice := make([]string, 0, len(env))
	for key, value := range env {
		envSlice = append(envSlice, fmt.Sprintf("%s=%s", key, value))
	}

	// Create command with environment
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Env = append(cmd.Environ(), envSlice...)
	cmd.Dir = path

	// Execute with the specified or default timeout
	return r.ExecuteWithTimeout(ctx, path, command, timeoutSeconds)
}

func (r *LinuxRuntime) StartProcess(
	ctx context.Context,
	path string,
	command []string,
	args []string,
) (string, error) {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)

	processID := uuid.New().String()

	// save process
	processInfo := &ProcessInfo{
		Id: processID,
		Cmd:    cmd,
		Name: "Process Name",
		Status: "running",
		Output: &bytes.Buffer{},
	}

	// capture output
	cmd.Stdout = processInfo.Output
	cmd.Stderr = processInfo.Output

	// start on background
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start: %w", err)
	}

	// save on processes list
	r.processLock.Lock()
	r.processes[processID] = processInfo
	r.processLock.Unlock()

	// monitoring process
	go func() {
		cmd.Wait()
		r.processLock.Lock()
		processInfo.Status = "finished"
		r.processLock.Unlock()
	}()

	return processID, nil
}

func (r *LinuxRuntime) StopProcess(
	ctx context.Context,
	processID string,
) error {
	// get process if exists
	r.processLock.RLock()
	process, exists := r.processes[processID]
	r.processLock.RUnlock()

	if !exists {
		return fmt.Errorf("process not found: %s", processID)
	}

	// stop process
	if err := process.Cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to stop: %w", err)
	}

	// set status
	process.Status = "stopped"
	return nil
}

func (r *LinuxRuntime) KillProcess(
	ctx context.Context,
	processID string,
) error {
	// get process if exists
	r.processLock.RLock()
	process, exists := r.processes[processID]
	r.processLock.RUnlock()

	if !exists {
		return fmt.Errorf("process not found: %s", processID)
	}

	// stop process
	if err := process.Cmd.Process.Kill(); err != nil {
		return fmt.Errorf("failed to kill: %w", err)
	}

	// set status
	process.Status = "stopped"
	return nil
}

func (r *LinuxRuntime) GetProcessStatus(ctx context.Context, processID string) (string, error) {
	r.processLock.RLock()
	processInfo, exists := r.processes[processID]
	r.processLock.RUnlock()

	if !exists {
		return "", fmt.Errorf("process not found: %s", processID)
	}

	return processInfo.Status, nil
}

func (r *LinuxRuntime) ListProcesses(
	ctx context.Context,
) ([]string, error) {
	processes := []string{}

	// check if its empty
	if len(r.processes) == 0 {
		return nil, fmt.Errorf("there are no process to list")
	}

	// save process
	for uuid := range r.processes {
		process := r.processes[uuid]

		// UUID - Name: Command [Status]
		content := fmt.Sprintf("%s - %s: %s [%s]\n", process.Id, process.Name, process.Cmd.String(), process.Status)


		processes = append(processes, content)
	}

	return processes, nil
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
	r.processLock.RLock()
	process, exists := r.processes[processID]
	r.processLock.RUnlock()

	if !exists {
		return fmt.Errorf("process not found: %s", processID)
	}

	// check if process is running
	if process.Status == "running" {
		stopErr := r.StopProcess(ctx, processID)

		if stopErr != nil {
			return stopErr
		}
	}

	// clean buffers
	if process.Output != nil {
		process.Output.Reset()
	}

	// delete process from map
	r.processLock.Lock()
	delete(r.processes, processID)
	r.processLock.Unlock()

	return nil
}

func (r *LinuxRuntime) CleanupAllProcesses(
	ctx context.Context,
) error {

	// clean each process
	for id := range r.processes {
		err := r.CleanupProcess(ctx, id)

		if err != nil {
			return err
		}
	}

	return nil
}

func (r *LinuxRuntime) Shutdown(
	ctx context.Context,
) error {

	// cleanup all processes
	if err := r.CleanupAllProcesses(ctx); err != nil {
		return err
	}

	// clean processes map
	r.processLock.Lock()
	r.processes = make(map[string]*ProcessInfo)
	r.processLock.Unlock()

	return nil
}
