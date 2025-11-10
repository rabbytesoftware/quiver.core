package runtime

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/rabbytesoftware/quiver/internal/core/watcher"
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

func (r *LinuxRuntime) ExecuteWithTimeout(ctx context.Context, path string, args []string, timeoutSeconds int) (string, error) {
	// set timeout
	ctxTimeout, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	// execute
	cmd := exec.CommandContext(ctxTimeout, args[0], args[1:]...)
	cmd.Dir = path
	out, err := cmd.CombinedOutput()

	// handler errors
	if ctxTimeout.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("execution timeout after %ds, aborting", timeoutSeconds)
	}
	if err != nil {
		return "", fmt.Errorf("linux exec error: %w", err)
	}
	return string(out), nil
}

func (r *LinuxRuntime) ExecuteWithEnvironment(
	ctx context.Context,
	command []string,
	env map[string]string,
) (string, error) {
	// set current directory as working
	// directory if not specified
	path := "."

	if len(command) == 0 {
		return "", fmt.Errorf("command cannot be empty")
	}

	// get timeout from environment
	// or use default
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

	// Execute with the specified
	// or default timeout
	return r.ExecuteWithTimeout(ctx, path, command, timeoutSeconds)
}

func (r *LinuxRuntime) StartProcess(
	ctx context.Context,
	path string,
	command []string,
	args []string,
) (string, error) {
	if len(command) == 0 {
		return "", fmt.Errorf("command cannot be empty")
	}

	// stream output and errors
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = path

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("stderr pipe: %w", err)
	}

	// initialize process
	id := uuid.New()
	pid := id.String()

	info := &ProcessInfo{
		Id:        id,
		Cmd:       cmd,
		Name:      strings.Join(command, " "),
		Status:    "running",
		Output:    &bytes.Buffer{},
		Error:     &bytes.Buffer{},
		OutChan:   make(chan string, 200),
		ErrorChan: make(chan string, 100),
	}

	// start process
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start: %w", err)
	}

	// Save in map
	r.processLock.Lock()
	if r.processes == nil {
		r.processes = make(map[string]*ProcessInfo)
	}
	r.processes[pid] = info
	r.processLock.Unlock()

	// read stdout
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()

			r.processLock.Lock()
			info.Output.WriteString(line + "\n")
			r.processLock.Unlock()

			select {
			case info.OutChan <- line:
			default:

				watcher.Warn(fmt.Sprintf("Output channel full for process %s: dropping line", pid))
			}
		}
	}()

	// read stderr
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			r.processLock.Lock()
			info.Error.WriteString(line + "\n")
			r.processLock.Unlock()

			select {
			case info.ErrorChan <- line:
			default:
			}
		}
	}()

	// monitor finish
	go func() {
		err := cmd.Wait()
		r.processLock.Lock()
		info.ExitErr = err
		if cmd.ProcessState != nil {
			info.ExitCode = cmd.ProcessState.ExitCode()
		}
		info.Status = "finished"
		r.processLock.Unlock()

		// close channels
		close(info.OutChan)
		close(info.ErrorChan)
	}()

	return pid, nil
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

	if process.Cmd == nil || process.Cmd.Process == nil {
		return fmt.Errorf("process has no underlying os.Process")
	}

	// stop process
	if err := process.Cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to stop: %w", err)
	}

	// set status
	r.processLock.Lock()
	process.Status = "stopping"
	r.processLock.Unlock()

	// wait until monitor marks
	// it finished or ctx timeout
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			r.processLock.RLock()
			st := process.Status
			r.processLock.RUnlock()
			if st != "running" && st != "stopping" {
				return nil // finished
			}
		}
	}
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

	if process.Cmd.Process == nil {
		return fmt.Errorf("something went wrong while trying to stop process")
	}

	// stop process
	if err := process.Cmd.Process.Kill(); err != nil {
		return fmt.Errorf("failed to kill: %w", err)
	}

	// set status
	r.processLock.Lock()
	process.Status = "killing"
	r.processLock.Unlock()

	// wait for monitor to set finished
	select {
	case <-ctx.Done():
		return ctx.Err()
		// timeout waiting for process to die
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout waiting for process to exit after kill")
	}
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
	r.processLock.RLock()
	process, exists := r.processes[processID]
	r.processLock.RUnlock()

	if !exists {
		return "", fmt.Errorf("process not found: %s", processID)
	}

	// capture process output
	r.processLock.RLock()
	output := process.Output.String()
	r.processLock.RUnlock()

	// save output log
	watcher.Info(output)

	return output, nil
}

func (r *LinuxRuntime) CaptureError(
	ctx context.Context,
	processID string,
) (string, error) {
	r.processLock.RLock()
	process, exists := r.processes[processID]
	r.processLock.RUnlock()

	if !exists {
		return "", fmt.Errorf("process not found: %s", processID)
	}

	// capture process output
	r.processLock.RLock()
	err := process.Error.String()
	exitCode := process.ExitCode
	r.processLock.RUnlock()

	errMessage := fmt.Sprintf("Error: %s. Exit with code: %d", err, exitCode)

	// save error log
	// watcher.Error(process.ExitErr)

	return errMessage, nil
}

func (r *LinuxRuntime) StreamOutput(
	ctx context.Context,
	processID string,
) (<-chan string, error) {
	// watcher
	r.processLock.RLock()
	process, exists := r.processes[processID]
	r.processLock.RUnlock()

	if !exists {
		return nil, fmt.Errorf("process not found: %s", processID)
	}

	// return process channel
	// (realtime)
	return process.OutChan, nil
}

func (r *LinuxRuntime) StreamError(
	ctx context.Context,
	processID string,
) (<-chan string, error) {
	r.processLock.RLock()
	process, exists := r.processes[processID]
	r.processLock.RUnlock()

	if !exists {
		return nil, fmt.Errorf("process not found: %s", processID)
	}

	// return process channel
	// (realtime)
	return process.ErrorChan, nil
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
