//go:build windows

package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/runtime/internal/models"
)

type windowsProcess struct {
	*baseProcess
}

func newProcess(
	ctx context.Context,
	config *models.Config,
) (Process, error) {
	if config.ShellWrap {
		joined := strings.Join(config.Command, " ")
		config = &models.Config{
			Command:     []string{"cmd.exe", "/C", joined},
			WorkDir:     config.WorkDir,
			Env:         config.Env,
			Timeout:     config.Timeout,
			KillTimeout: config.KillTimeout,
			StopTimeout: config.StopTimeout,
			BufferSize:  config.BufferSize,
		}
	}

	base, err := newBaseProcess(ctx, config)
	if err != nil {
		return nil, err
	}

	p := &windowsProcess{baseProcess: base}

	if err := p.baseProcess.startCommon(ctx); err != nil {
		return nil, err
	}

	return p, nil
}

func (p *windowsProcess) Stop(
	ctx context.Context,
) error {
	p.mu.RLock()
	currentStatus := p.status
	timeout := p.config.StopTimeout
	p.mu.RUnlock()

	if currentStatus != models.StatusRunning {
		return models.ErrInvalidState
	}
	if p.cmd == nil || p.cmd.Process == nil {
		return models.ErrNoProcess
	}

	p.setStatus(models.StatusStopping)

	// Windows has no graceful SIGTERM; Kill is used with Stopping status
	// to distinguish a requested stop from a forced kill.
	if err := p.cmd.Process.Kill(); err != nil {
		if isProcessGone(err) {
			select {
			case <-p.done:
				return nil
			case <-time.After(timeout):
				return nil
			}
		}
		if p.Status() == models.StatusFinished {
			return nil
		}
		return fmt.Errorf("failed to stop: %w", err)
	}

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
			return fmt.Errorf("stop timeout after %v", timeout)
		}
		return timeoutCtx.Err()
	}
}

func (p *windowsProcess) Kill(
	ctx context.Context,
) error {
	p.mu.RLock()
	timeout := p.config.KillTimeout
	p.mu.RUnlock()

	if p.cmd == nil || p.cmd.Process == nil {
		return models.ErrNoProcess
	}

	p.setStatus(models.StatusKilling)

	if err := p.cmd.Process.Kill(); err != nil {
		if isProcessGone(err) {
			select {
			case <-p.done:
				return nil
			case <-time.After(timeout):
				return nil
			}
		}
		if p.Status() == models.StatusFinished {
			return nil
		}
		return fmt.Errorf("failed to kill: %w", err)
	}

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
			return fmt.Errorf("%w: timeout after %v", models.ErrKillTimeout, timeout)
		}
		return timeoutCtx.Err()
	}
}

// Interrupt falls back to Kill since Windows has no SIGINT equivalent.
func (p *windowsProcess) Interrupt(
	ctx context.Context,
) error {
	return p.Kill(ctx)
}

func isAlive(_ int) bool {
	return false
}

func signalPID(
	_ context.Context,
	pid int,
	sig domainstep.SignalKind,
) error {
	if pid <= 0 {
		return fmt.Errorf("invalid PID: %d", pid)
	}

	switch sig {
	case domainstep.SignalKindKill, domainstep.SignalKindGraceful, domainstep.SignalKindInterrupt:
		// Windows has no SIGTERM equivalent; all signal kinds use force-kill (/F).
		cmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("signalPID %d: %w", pid, err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported signal: %s", sig)
	}
}

func isProcessGone(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.EINVAL) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "invalid argument") ||
		strings.Contains(msg, "access is denied") ||
		strings.Contains(msg, "process already finished")
}
