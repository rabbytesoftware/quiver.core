package runtime

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rabbytesoftware/quiver/internal/core/watcher"
)

type WindowsRuntime struct {
	*Runtime
	processes   map[string]*ProcessInfo
	processLock sync.RWMutex
}

func (r *WindowsRuntime) Execute(ctx context.Context, path string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, "cmd", append([]string{"/C"}, args...)...)
	cmd.Dir = path

	// wait until execution ends
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("windows exec error: %w", err)
	}

	return string(out), nil
}

func (r *WindowsRuntime) ExecuteWithTimeout(ctx context.Context, path string, args []string, timeoutSeconds int) (string, error) {
	// set timeout
	ctxTimeout, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	// execute
	cmd := exec.CommandContext(ctxTimeout, "cmd", append([]string{"/C"}, args...)...)
	cmd.Dir = path
	out, err := cmd.CombinedOutput()

	// handler errors
	if ctxTimeout.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("execution timeout after %ds, aborting", timeoutSeconds)
	}
	if err != nil {
		return "", fmt.Errorf("windows exec error: %w", err)
	}
	return string(out), nil
}

func (r *WindowsRuntime) ExecuteWithEnvironment(
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
	for k, v := range env {
		envSlice = append(envSlice, fmt.Sprintf("%s=%s", k, v))
	}

	// Create command with environment
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Env = append(cmd.Environ(), envSlice...)
	cmd.Dir = path

	// Execute with the specified
	// or default timeout
	return r.ExecuteWithTimeout(ctx, path, command, timeoutSeconds)
}

func (r *WindowsRuntime) StartProcess(
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
		DoneChan:  make(chan struct{}),
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

		for {
			select {
			case <-ctx.Done():
				// exit
				return
			default:
				if !scanner.Scan() {
					// EOF or error → exit
					return
				}
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
		}
	}()

	// read stderr
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for {
			select {
			case <-ctx.Done():
				return // exit
			default:
				if !scanner.Scan() {
					// EOF or error → exit
					return
				}

				line := scanner.Text()

				// save err
				r.processLock.Lock()
				info.Error.WriteString(line + "\n")
				r.processLock.Unlock()

				select {
				case info.ErrorChan <- line:
				default:
					watcher.Warn(fmt.Sprintf("Error channel full for process %s: dropping line", pid))
				}
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
		close(info.OutChan)
		close(info.ErrorChan)
		close(info.DoneChan)
	}()

	return pid, nil
}

func (r *WindowsRuntime) StopProcess(
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
	if err := process.Cmd.Process.Kill(); err != nil {
		return fmt.Errorf("failed to stop: %w", err)
	}

	// set status
	r.processLock.Lock()
	process.Status = "stopping"
	r.processLock.Unlock()

	// wait until monitor marks it
	// finished or ctx timeout
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-process.DoneChan:
		return nil // process finished
	}
}

func (r *WindowsRuntime) KillProcess(
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
		return fmt.Errorf("something went wrong while trying to stop process")
	}

	// kill process
	if err := process.Cmd.Process.Kill(); err != nil {
		if errors.Is(err, os.ErrProcessDone) || strings.Contains(err.Error(), "already finished") {
		} else {
			return fmt.Errorf("failed to kill: %w", err)
		}
	}

	// set status
	r.processLock.Lock()
	process.Status = "killing"
	r.processLock.Unlock()

	// wait for monitor to set finished
	select {
	case <-process.DoneChan:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timeout waiting for process to exit after kill")
	}
}

func (r *WindowsRuntime) GetProcessStatus(ctx context.Context, processID string) (string, error) {
	r.processLock.RLock()
	processInfo, exists := r.processes[processID]
	r.processLock.RUnlock()

	if !exists {
		return "", fmt.Errorf("process not found: %s", processID)
	}

	return processInfo.Status, nil
}

func (r *WindowsRuntime) ListProcesses(
	ctx context.Context,
) ([]string, error) {
	processes := []string{}

	// check if its empty
	if len(r.processes) == 0 {
		return nil, fmt.Errorf("there are no process to list")
	}

	// save process
	for uid := range r.processes {
		process := r.processes[uid]

		// UUID - Name: Command [Status]
		content := fmt.Sprintf("%s - %s: %s [%s]\n", process.Id, process.Name, process.Cmd.String(), process.Status)
		processes = append(processes, content)
	}

	return processes, nil
}

func (r *WindowsRuntime) CaptureOutput(
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

	return output, nil
}

func (r *WindowsRuntime) CaptureError(
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

	return errMessage, nil
}

func (r *WindowsRuntime) StreamOutput(
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

	return process.OutChan, nil
}

func (r *WindowsRuntime) StreamError(
	ctx context.Context,
	processID string,
) (<-chan string, error) {
	r.processLock.RLock()
	process, exists := r.processes[processID]
	r.processLock.RUnlock()

	if !exists {
		return nil, fmt.Errorf("process not found: %s", processID)
	}

	return process.ErrorChan, nil
}

func (r *WindowsRuntime) CleanupProcess(
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
		process.Error.Reset()
	}

	// delete process from map
	r.processLock.Lock()
	delete(r.processes, processID)
	r.processLock.Unlock()

	return nil
}

func (r *WindowsRuntime) CleanupAllProcesses(ctx context.Context) error {
	var errs []error

	// copy process IDs
	r.processLock.RLock()
	ids := make([]string, 0, len(r.processes))
	for id := range r.processes {
		ids = append(ids, id)
	}
	r.processLock.RUnlock()

	// clean each process
	for _, id := range ids {
		if err := r.CleanupProcess(ctx, id); err != nil {
			errs = append(errs, fmt.Errorf("failed to cleanup %s: %w", id, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("cleanup completed with %d error(s): %v", len(errs), errs)
	}

	return nil
}

func (r *WindowsRuntime) Shutdown(
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
