//go:build darwin

package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/rabbytesoftware/quiver/internal/engines/wizard/runtime/models"
)

type DarwinProcess struct {
	*BaseProcess
}

func NewDarwinProcess(ctx context.Context, config *models.Config) (*DarwinProcess, error) {
	base, err := NewBaseProcess(ctx, config)
	if err != nil {
		return nil, err
	}

	// Set up process group for proper child process management
	base.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	return &DarwinProcess{
		BaseProcess: base,
	}, nil
}

func (p *DarwinProcess) Start(ctx context.Context) error {
	return p.StartCommon(ctx)
}

func (p *DarwinProcess) Stop(ctx context.Context) error {
	p.mu.RLock()
	currentStatus := p.status
	stopTimeout := p.config.StopTimeout
	p.mu.RUnlock()

	if currentStatus != models.StatusRunning {
		return models.ErrInvalidState
	}

	if p.cmd.Process == nil {
		return models.ErrNoProcess
	}

	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to stop: %w", err)
	}

	p.SetStatus(models.StatusStopping)

	// Create timeout context if we have a stop timeout
	var timeoutCtx context.Context
	var cancel context.CancelFunc
	if stopTimeout > 0 {
		timeoutCtx, cancel = context.WithTimeout(ctx, stopTimeout)
		defer cancel()
	} else {
		timeoutCtx = ctx
	}

	select {
	case <-p.doneChan:
		return nil
	case <-timeoutCtx.Done():
		if stopTimeout > 0 && timeoutCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("stop timeout after %v", stopTimeout)
		}
		return timeoutCtx.Err()
	}
}

func (p *DarwinProcess) Kill(ctx context.Context) error {
	p.mu.RLock()
	killTimeout := p.config.KillTimeout
	p.mu.RUnlock()

	if p.cmd.Process == nil {
		return models.ErrNoProcess
	}

	if err := p.cmd.Process.Kill(); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			select {
			case <-p.doneChan:
				return nil
			case <-time.After(30 * time.Second):
				return nil
			}
		}
		return fmt.Errorf("failed to kill: %w", err)
	}

	p.SetStatus(models.StatusKilling)

	// Create timeout context if we have a kill timeout
	var timeoutCtx context.Context
	var cancel context.CancelFunc
	if killTimeout > 0 {
		timeoutCtx, cancel = context.WithTimeout(ctx, killTimeout)
		defer cancel()
	} else {
		timeoutCtx = ctx
	}

	select {
	case <-p.doneChan:
		return nil
	case <-timeoutCtx.Done():
		if killTimeout > 0 && timeoutCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("%w: timeout after %v", models.ErrKillTimeout, killTimeout)
		}
		return timeoutCtx.Err()
	}
}
