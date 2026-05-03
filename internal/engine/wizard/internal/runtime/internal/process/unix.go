//go:build darwin || linux

package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/wizard/internal/runtime/internal/models"
)

type unixProcess struct {
	*baseProcess
}

func newProcess(
	ctx context.Context,
	config *models.Config,
) (Process, error) {
	if config.ShellWrap {
		joined := strings.Join(config.Command, " ")
		config = &models.Config{
			Command:     []string{"sh", "-c", joined},
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

	base.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	p := &unixProcess{baseProcess: base}

	if err := p.baseProcess.startCommon(ctx); err != nil {
		return nil, err
	}

	return p, nil
}

func (p *unixProcess) Stop(
	ctx context.Context,
) error {
	p.mu.RLock()
	currentStatus := p.status
	timeout := p.config.StopTimeout
	p.mu.RUnlock()

	if currentStatus != models.StatusRunning {
		return models.ErrInvalidState
	}

	return p.stopWithTimeout(
		ctx,
		timeout,
		func() error {
			if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
				return fmt.Errorf("failed to stop: %w", err)
			}
			return nil
		},
		models.StatusStopping,
	)
}

func (p *unixProcess) Kill(
	ctx context.Context,
) error {
	p.mu.RLock()
	timeout := p.config.KillTimeout
	p.mu.RUnlock()

	return p.stopWithTimeout(
		ctx,
		timeout,
		func() error {
			if err := p.cmd.Process.Kill(); err != nil {
				if errors.Is(err, os.ErrProcessDone) {
					select {
					case <-p.done:
						return nil
					case <-time.After(timeout):
						return nil
					}
				}
				return fmt.Errorf("failed to kill: %w", err)
			}
			return nil
		},
		models.StatusKilling,
	)
}

func (p *unixProcess) Interrupt(
	ctx context.Context,
) error {
	p.mu.RLock()
	timeout := p.config.StopTimeout
	p.mu.RUnlock()

	return p.stopWithTimeout(
		ctx,
		timeout,
		func() error {
			if err := p.cmd.Process.Signal(syscall.SIGINT); err != nil {
				return fmt.Errorf("failed to interrupt: %w", err)
			}
			return nil
		},
		models.StatusStopping,
	)
}

func isAlive(pid int) bool {
	return pid > 0 && syscall.Kill(pid, 0) == nil
}

func signalPID(
	_ context.Context,
	pid int,
	sig domainstep.SignalKind,
) error {
	if pid <= 0 {
		return fmt.Errorf("invalid PID: %d", pid)
	}

	var osSig syscall.Signal
	switch sig {
	case domainstep.SignalKindGraceful:
		osSig = syscall.SIGTERM
	case domainstep.SignalKindKill:
		osSig = syscall.SIGKILL
	case domainstep.SignalKindInterrupt:
		osSig = syscall.SIGINT
	default:
		return fmt.Errorf("unsupported signal: %s: %w", sig, models.ErrInvalidSignal)
	}

	if err := syscall.Kill(pid, osSig); err != nil {
		return fmt.Errorf("signalPID %d: %w", pid, err)
	}
	return nil
}
