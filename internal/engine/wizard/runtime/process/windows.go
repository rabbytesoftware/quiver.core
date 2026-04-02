//go:build windows

package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/models"
)

type WindowsProcess struct {
	*BaseProcess
}

func NewWindowsProcess(ctx context.Context, config *models.Config) (*WindowsProcess, error) {
	base, err := NewBaseProcess(ctx, config)
	if err != nil {
		return nil, err
	}

	return &WindowsProcess{
		BaseProcess: base,
	}, nil
}

func (p *WindowsProcess) Start(ctx context.Context) error {
	return p.StartCommon(ctx)
}

func (p *WindowsProcess) Signal(sig os.Signal) error {
	return fmt.Errorf("signal not supported on windows")
}

func (p *WindowsProcess) Stop(ctx context.Context) error {
	p.mu.RLock()
	currentStatus := p.status
	stopTimeout := p.config.StopTimeout
	p.mu.RUnlock()

	if currentStatus != models.StatusRunning {
		return models.ErrInvalidState
	}

	if p.cmd == nil || p.cmd.Process == nil {
		return models.ErrNoProcess
	}

	p.SetStatus(models.StatusStopping)

	// Windows doesn't support graceful termination signals like SIGTERM
	// We use Kill() which sends SIGKILL, but we set status to Stopping
	// to indicate this was a requested stop rather than a forced kill
	if err := p.cmd.Process.Kill(); err != nil {
		if isProcessGoneError(err) {
			select {
			case <-p.doneChan:
				return nil
			case <-time.After(stopTimeout):
				return nil
			}
		}
		p.mu.RLock()
		status := p.status
		p.mu.RUnlock()
		if status == models.StatusFinished {
			return nil
		}
		return fmt.Errorf("failed to stop: %w", err)
	}

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

func (p *WindowsProcess) Kill(ctx context.Context) error {
	p.mu.RLock()
	killTimeout := p.config.KillTimeout
	p.mu.RUnlock()

	if p.cmd == nil || p.cmd.Process == nil {
		return models.ErrNoProcess
	}

	p.SetStatus(models.StatusKilling)

	if err := p.cmd.Process.Kill(); err != nil {
		if isProcessGoneError(err) {
			select {
			case <-p.doneChan:
				return nil
			case <-time.After(killTimeout):
				return nil
			}
		}
		p.mu.RLock()
		status := p.status
		p.mu.RUnlock()
		if status == models.StatusFinished {
			return nil
		}
		return fmt.Errorf("failed to kill: %w", err)
	}

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

func isProcessGoneError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, os.ErrProcessDone) {
		return true
	}

	if errors.Is(err, syscall.EINVAL) {
		return true
	}

	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "invalid argument") ||
		strings.Contains(errStr, "access is denied") ||
		strings.Contains(errStr, "process already finished")
}
