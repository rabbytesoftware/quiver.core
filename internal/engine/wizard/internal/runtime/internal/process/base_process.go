package process

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/runtime/internal/models"
)

// outputHandler buffers process stdout/stderr and exposes streaming channels.
type outputHandler struct {
	output  *bytes.Buffer
	errBuf  *bytes.Buffer
	outChan chan string
	errChan chan string
	mu      sync.RWMutex
	closed  bool
	closeMu sync.Mutex
}

func newOutputHandler(
	outChanSize int,
	errChanSize int,
) *outputHandler {
	return &outputHandler{
		output:  &bytes.Buffer{},
		errBuf:  &bytes.Buffer{},
		outChan: make(chan string, outChanSize),
		errChan: make(chan string, errChanSize),
	}
}

func (h *outputHandler) writeOutput(
	line string,
) {
	h.mu.Lock()
	h.output.WriteString(line + "\n")
	h.mu.Unlock()

	h.closeMu.Lock()
	defer h.closeMu.Unlock()

	if h.closed {
		return
	}
	select {
	case h.outChan <- line:
	default:
		slog.Warn("output channel full, dropping line")
	}
}

func (h *outputHandler) writeError(
	line string,
) {
	h.mu.Lock()
	h.errBuf.WriteString(line + "\n")
	h.mu.Unlock()

	h.closeMu.Lock()
	defer h.closeMu.Unlock()

	if h.closed {
		return
	}
	select {
	case h.errChan <- line:
	default:
		slog.Warn("error channel full, dropping line")
	}
}

func (h *outputHandler) getOutput() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.output.String()
}

func (h *outputHandler) getError() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.errBuf.String()
}

func (h *outputHandler) close() {
	h.closeMu.Lock()
	defer h.closeMu.Unlock()

	if h.closed {
		return
	}
	h.closed = true
	close(h.outChan)
	close(h.errChan)
}

type baseProcess struct {
	id       string
	cmd      *exec.Cmd
	config   *models.Config
	status   models.Status
	exitCode int
	output   *outputHandler
	done     chan struct{}
	mu       sync.RWMutex
}

func newBaseProcess(
	ctx context.Context,
	config *models.Config,
) (*baseProcess, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// #nosec G204 -- command is validated before execution
	cmd := exec.CommandContext(ctx, config.Command[0], config.Command[1:]...)
	cmd.Dir = config.WorkDir

	if len(config.Env) > 0 {
		cmd.Env = append(cmd.Environ(), envMapToSlice(config.Env)...)
	}

	outSize := 200
	errSize := 100
	if config.BufferSize > 0 {
		outSize = config.BufferSize
		errSize = config.BufferSize / 2
	}

	return &baseProcess{
		id:       uuid.New().String(),
		cmd:      cmd,
		config:   config,
		status:   models.StatusPrepared,
		exitCode: -1,
		output:   newOutputHandler(outSize, errSize),
		done:     make(chan struct{}),
	}, nil
}

func (p *baseProcess) ID() string { return p.id }

func (p *baseProcess) PID() int {
	if p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *baseProcess) Status() models.Status {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

func (p *baseProcess) ExitCode() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.exitCode
}

func (p *baseProcess) Done() <-chan struct{} { return p.done }

func (p *baseProcess) Output() string              { return p.output.getOutput() }
func (p *baseProcess) Error() string               { return p.output.getError() }
func (p *baseProcess) StreamOutput() <-chan string { return p.output.outChan }
func (p *baseProcess) StreamError() <-chan string  { return p.output.errChan }

func (p *baseProcess) setStatus(
	status models.Status,
) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = status
}

func (p *baseProcess) Wait(
	ctx context.Context,
) error {
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *baseProcess) Close() error {
	p.output.close()
	return nil
}

func (p *baseProcess) startCommon() error {
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

	p.setStatus(models.StatusRunning)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdoutPipe)
		scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
		for scanner.Scan() {
			p.output.writeOutput(scanner.Text())
		}
	}()

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderrPipe)
		scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
		for scanner.Scan() {
			p.output.writeError(scanner.Text())
		}
	}()

	go func() {
		wg.Wait()
		_ = p.cmd.Wait() // #nosec G104 -- exit code is read from ProcessState below

		p.mu.Lock()
		if p.cmd.ProcessState != nil {
			p.exitCode = p.cmd.ProcessState.ExitCode()
		}
		p.status = models.StatusFinished
		p.mu.Unlock()

		p.output.close()
		close(p.done)
	}()

	return nil
}

func (p *baseProcess) stopWithTimeout(
	ctx context.Context,
	timeout time.Duration,
	send func() error,
	transitioning models.Status,
) error {
	if err := send(); err != nil {
		return err
	}

	p.setStatus(transitioning)

	timeoutCtx := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		timeoutCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	select {
	case <-p.done:
		return nil
	case <-timeoutCtx.Done():
		if timeout > 0 && timeoutCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("timeout after %v: %w", timeout, models.ErrKillTimeout)
		}
		return timeoutCtx.Err()
	}
}

func envMapToSlice(
	env map[string]string,
) []string {
	if env == nil {
		return nil
	}
	slice := make([]string, 0, len(env))
	for k, v := range env {
		slice = append(slice, fmt.Sprintf("%s=%s", k, v))
	}
	return slice
}
