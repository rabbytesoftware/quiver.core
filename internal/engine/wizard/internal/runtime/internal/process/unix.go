//go:build darwin || linux

package process

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"time"

	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/runtime/internal/models"
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

	// exec.CommandContext's own cancellation kills the single spawned PID,
	// which leaks exactly what signalGroup exists to prevent: a cancelled
	// one-shot step — the only kind wizard.Shutdown cancels — would leave
	// whatever its shell forked still running. ESRCH here only means the
	// process beat the cancellation to it, which is the outcome cancelling
	// wanted anyway.
	base.cmd.Cancel = func() error {
		if err := signalGroup(base.PID(), syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
		return nil
	}

	p := &unixProcess{baseProcess: base}

	if err := p.startCommon(); err != nil {
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
			if err := signalGroup(p.PID(), syscall.SIGTERM); err != nil {
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
			if err := signalGroup(p.PID(), syscall.SIGKILL); err != nil {
				// ESRCH is this path's os.ErrProcessDone: signalling the group
				// directly bypasses os.Process's own "already waited for"
				// bookkeeping, so a process that already exited and was reaped
				// surfaces as a missing group rather than os.ErrProcessDone.
				// Either way the caller asked for a dead process and gets one.
				if errors.Is(err, syscall.ESRCH) {
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
			if err := signalGroup(p.PID(), syscall.SIGINT); err != nil {
				return fmt.Errorf("failed to interrupt: %w", err)
			}
			return nil
		},
		models.StatusStopping,
	)
}

// signalGroup delivers sig to the whole process group led by pid.
//
// newProcess puts every process it spawns into a group of its own (Setpgid
// with Pgid left at zero), so the spawned PID is always that group's leader
// and the negative-PID form reaches it together with everything it forked.
// Signalling the bare PID is not enough once a step is shell-wrapped: /bin/sh
// only exec-optimizes itself into a single simple command, and for anything
// else (`a & wait`, `a; b`, `a && b`) it forks a child and stays on as its
// supervisor. Killing just the shell then orphans that child, which keeps the
// stdout/stderr pipe write-ends it inherited open — so startCommon's scanners
// never reach EOF, cmd.Wait is never called, the shell is never reaped, and
// isAlive keeps reporting its zombie as running for as long as the orphan
// lives. Group-signalling a group of one behaves identically to signalling
// that one PID, so this is the correct call for every spawned process.
func signalGroup(pid int, sig syscall.Signal) error {
	if pid <= 0 {
		return fmt.Errorf("invalid PID: %d", pid)
	}
	return syscall.Kill(-pid, sig)
}

// isAlive reports whether pid is still in the process table. A zombie counts
// as alive: signal 0 succeeds against one until its parent reaps it, which is
// exactly why nothing here may leave a spawned process unreaped — see
// signalGroup.
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

	// Every PID that reaches here was emitted by run's own step handler from a
	// process newProcess spawned, so it is always a group leader and the
	// group-wide delivery signalGroup performs is the one a lifecycle `stop`
	// step means: reach the step's shell and whatever it forked, not the
	// wrapper alone.
	if err := signalGroup(pid, osSig); err != nil {
		return fmt.Errorf("signalPID %d: %w", pid, err)
	}
	return nil
}
