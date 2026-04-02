package process

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/models"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/output"
)

// Process defines the interface for process lifecycle management
type Process interface {
	ID() string
	Key() string
	PID() int

	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Kill(ctx context.Context) error
	Wait(ctx context.Context) error
	Signal(sig os.Signal) error
	Done() <-chan struct{}

	Close() error
	Status() models.Status
	ExitCode() int

	Output() string
	Error() string
	StreamOutput() <-chan string
	StreamError() <-chan string
}

type BaseProcess struct {
	id            string
	key           string
	cmd           *exec.Cmd
	config        *models.Config
	status        models.Status
	exitCode      int
	outputHandler *output.Handler
	doneChan      chan struct{}
	mu            sync.RWMutex
}

func NewBaseProcess(ctx context.Context, config *models.Config) (*BaseProcess, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// #nosec G204 -- Command is validated through config.Validate() before execution
	cmd := exec.CommandContext(ctx, config.Command[0], config.Command[1:]...)
	cmd.Dir = config.WorkDir

	if len(config.Env) > 0 {
		envSlice := envMapToSlice(config.Env)
		cmd.Env = append(cmd.Environ(), envSlice...)
	}

	outChanSize := 200
	errChanSize := 100
	if config.BufferSize > 0 {
		outChanSize = config.BufferSize
		errChanSize = config.BufferSize / 2
	}

	return &BaseProcess{
		id:            uuid.New().String(),
		cmd:           cmd,
		config:        config,
		status:        models.StatusPrepared,
		exitCode:      -1,
		outputHandler: output.NewHandlerWithBuffers(outChanSize, errChanSize),
		doneChan:      make(chan struct{}),
	}, nil
}

func (p *BaseProcess) ID() string {
	return p.id
}

func (p *BaseProcess) Key() string {
	return p.key
}

func (p *BaseProcess) PID() int {
	if p.cmd.Process == nil {
		return -1
	}
	return p.cmd.Process.Pid
}

func (p *BaseProcess) Done() <-chan struct{} {
	return p.doneChan
}

func (p *BaseProcess) Status() models.Status {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

func (p *BaseProcess) ExitCode() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.exitCode
}

func (p *BaseProcess) Output() string {
	return p.outputHandler.GetOutput()
}

func (p *BaseProcess) Error() string {
	return p.outputHandler.GetError()
}

// StreamOutput returns a channel that emits stdout lines in real time.
// Lines are dropped when the buffer is full — callers must drain this channel
// actively or data will be lost. Use WithBufferSize on the builder to increase
// the buffer capacity for high-output processes.
func (p *BaseProcess) StreamOutput() <-chan string {
	return p.outputHandler.OutChan()
}

// StreamError returns a channel that emits stderr lines in real time.
// Lines are dropped when the buffer is full — callers must drain this channel
// actively or data will be lost. Use WithBufferSize on the builder to increase
// the buffer capacity for high-output processes.
func (p *BaseProcess) StreamError() <-chan string {
	return p.outputHandler.ErrChan()
}

func (p *BaseProcess) GetConfig() *models.Config {
	return p.config
}

func (p *BaseProcess) GetCmd() *exec.Cmd {
	return p.cmd
}

func (p *BaseProcess) GetOutputHandler() *output.Handler {
	return p.outputHandler
}

func (p *BaseProcess) GetDoneChan() chan struct{} {
	return p.doneChan
}

func (p *BaseProcess) SetStatus(status models.Status) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = status
}

func (p *BaseProcess) SetExitCode(code int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.exitCode = code
}

func (p *BaseProcess) Wait(ctx context.Context) error {
	select {
	case <-p.doneChan:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *BaseProcess) Close() error {
	p.outputHandler.Close()
	return nil
}

func (p *BaseProcess) StartCommon(ctx context.Context) error {
	p.mu.Lock()
	if p.status != models.StatusPrepared {
		p.mu.Unlock()
		return models.ErrInvalidState
	}
	p.mu.Unlock()

	stdoutPipe, err := p.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	stderrPipe, err := p.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start: %w", err)
	}

	pid := p.cmd.Process.Pid
	keyInput := fmt.Sprintf("%d-%d", pid, time.Now().UnixNano())
	p.key = uuid.NewSHA1(uuid.NameSpaceOID, []byte(keyInput)).String()

	p.SetStatus(models.StatusRunning)

	// Use WaitGroup to ensure output goroutines finish before closing handler
	var outputWg sync.WaitGroup
	outputWg.Add(2)

	// Output reader goroutine
	go func() {
		defer outputWg.Done()
		scanner := bufio.NewScanner(stdoutPipe)
		scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
				line := scanner.Text()
				p.outputHandler.WriteOutput(line)
			}
		}
	}()

	// Error reader goroutine
	go func() {
		defer outputWg.Done()
		scanner := bufio.NewScanner(stderrPipe)
		scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
				line := scanner.Text()
				p.outputHandler.WriteError(line)
			}
		}
	}()

	// Wait goroutine
	go func() {
		waitErr := p.cmd.Wait()

		// Wait for output goroutines to finish draining pipes
		outputWg.Wait()

		p.mu.Lock()
		if p.cmd.ProcessState != nil {
			p.exitCode = p.cmd.ProcessState.ExitCode()
		}
		p.status = models.StatusFinished
		p.mu.Unlock()

		p.outputHandler.Close()
		close(p.doneChan)

		// Log the error if process failed
		if waitErr != nil && p.exitCode != 0 {
			// Error is handled via exit code, which is checked by caller
		}
	}()

	return nil
}

func envMapToSlice(env map[string]string) []string {
	if env == nil {
		return nil
	}

	slice := make([]string, 0, len(env))
	for k, v := range env {
		slice = append(slice, fmt.Sprintf("%s=%s", k, v))
	}
	return slice
}
