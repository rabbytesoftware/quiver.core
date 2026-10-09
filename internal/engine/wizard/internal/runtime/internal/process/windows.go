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

	"golang.org/x/sys/windows"

	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/runtime/internal/models"
)

// createNoWindow keeps a console program from opening a cmd window of its own;
// every step runs under the daemon, which has no console to share.
const createNoWindow = 0x08000000

type windowsProcess struct {
	*baseProcess
}

func newProcess(
	ctx context.Context,
	config *models.Config,
) (Process, error) {
	shellWrapped := config.ShellWrap

	var cmdLine string
	if shellWrapped {
		cmdLine = windowsShellCommandLine(config.Command)
		joined := strings.Join(config.Command, " ")
		config = &models.Config{
			Command:     []string{windowsShell, windowsShellFlag, joined},
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

	// Command still resolves cmd.exe into the application name CreateProcess is
	// given, but the line cmd itself re-parses comes from here. A step that is
	// not shell-wrapped is left alone: it runs an executable directly, and Go's
	// escaping is the right escaping for that.
	base.cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	if shellWrapped {
		base.cmd.SysProcAttr.CmdLine = cmdLine
	}

	// exec.CommandContext's own cancellation kills the single spawned PID,
	// which for a shell-wrapped step is only cmd.exe: whatever it launched
	// would outlive the step. See killTree.
	base.cmd.Cancel = func() error {
		pid := base.PID()
		if err := killTree(pid); err != nil && !isProcessGone(err) {
			return base.cmd.Process.Kill()
		}
		return nil
	}

	p := &windowsProcess{baseProcess: base}

	if err := p.baseProcess.startCommon(); err != nil {
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

	// Windows has no graceful SIGTERM; a tree kill is used with Stopping
	// status to distinguish a requested stop from a forced kill.
	if err := p.terminate(); err != nil {
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

	if err := p.terminate(); err != nil {
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

// terminate force-kills the process together with everything it started.
//
// A shell-wrapped step is cmd.exe /C <command>: cmd stays on as the parent of
// the program it launched, so killing only the spawned PID orphans that
// program, which then keeps running (and holding whatever it bound, such as
// an arrow's ui socket) after its run has ended. Windows has no process
// groups, so the tree is walked by taskkill /T. If the tree kill itself
// fails, the spawned process alone is killed, as before.
func (p *windowsProcess) terminate() error {
	if err := killTree(p.cmd.Process.Pid); err == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

// killTree force-kills pid and its descendants.
func killTree(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid PID: %d", pid)
	}

	cmd := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)) // #nosec -- pid is validated > 0 above, never caller-controlled string input
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("taskkill %d: %w: %s", pid, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Interrupt falls back to Kill since Windows has no SIGINT equivalent.
func (p *windowsProcess) Interrupt(
	ctx context.Context,
) error {
	return p.Kill(ctx)
}

// isAlive reports whether pid is a running process. Crash recovery asks it for
// the process a previous daemon started: answering false for a process that is
// still running makes recovery treat its arrow as dead and leaves the process
// running with nothing tracking it.
func isAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid)) // #nosec G115 -- a pid from the OS fits in 32 bits
	if err != nil {
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer windows.CloseHandle(handle) //nolint:errcheck // nothing to do about a failed close

	state, err := windows.WaitForSingleObject(handle, 0)
	return err == nil && state == uint32(windows.WAIT_TIMEOUT)
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
		// Windows has no SIGTERM equivalent; all signal kinds use force-kill (/F),
		// over the whole tree (/T) like the group delivery on unix.
		if err := killTree(pid); err != nil {
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
