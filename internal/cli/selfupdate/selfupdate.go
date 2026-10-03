// Package selfupdate replaces the running daemon's binary with a newer one:
// it asks the daemon to shut down, swaps the binary at the self path, starts
// the new daemon with the same arguments and rolls back to the old binary when
// the new one does not come up healthy.
//
// It runs as a detached process (see Detach), because the daemon that started
// it is the very thing it has to stop.
package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/cli/client"
	"github.com/rabbytesoftware/quiver.core/internal/core/gateway"
	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
)

const (
	// stopTimeout covers the daemon's shutdown phases, whose budgets add up to
	// about 54s.
	stopTimeout   = 90 * time.Second
	healthTimeout = 30 * time.Second
	// killTimeout is how long a killed daemon gets to disappear before its
	// binary is replaced. A zombie still counts as alive until its parent, this
	// process, exits, so it is not waited on for long.
	killTimeout  = 5 * time.Second
	pollInterval = 200 * time.Millisecond
)

// Updater holds what a swap needs from the machine, so a test can replace the
// processes with fakes.
type Updater struct {
	// Self is the path the daemon is started from.
	Self string
	// Shutdown asks the running daemon to stop and names it.
	Shutdown func(ctx context.Context) (dto.ShutdownDTO, error)
	// Alive reports whether a process exists.
	Alive func(pid int) bool
	// Listening reports whether anything accepts connections on the socket.
	Listening func(ctx context.Context) bool
	// Healthy reports whether the daemon answers its health check.
	Healthy func(ctx context.Context) bool
	// Start launches a detached daemon and returns its pid.
	Start func(exe string, args []string) (int, error)
	// Kill ends a process this updater started.
	Kill func(pid int)
	// Started, when set, is told the pid of every daemon left running.
	Started func(pid int)
	// Socket names the socket Shutdown talks to, for error messages.
	Socket string

	StopTimeout   time.Duration
	HealthTimeout time.Duration
	Poll          time.Duration
}

// New builds an Updater for the daemon listening on socket and started from
// self.
func New(
	socket string,
	self string,
) (*Updater, error) {
	c, err := client.New(gateway.LocalURI(socket))
	if err != nil {
		return nil, fmt.Errorf("selfupdate: client for %s: %w", socket, err)
	}

	return &Updater{
		Self:     self,
		Socket:   socket,
		Shutdown: c.Shutdown,
		Alive:    alive,
		Listening: func(ctx context.Context) bool {
			conn, err := gateway.Dial(ctx, socket)
			if err != nil {
				return false
			}
			_ = conn.Close()
			return true
		},
		Healthy: func(ctx context.Context) bool { return c.Health(ctx) == nil },
		Start:   startDetached,
		Kill: func(pid int) {
			if proc, err := os.FindProcess(pid); err == nil {
				_ = proc.Kill()
			}
		},
		StopTimeout:   stopTimeout,
		HealthTimeout: healthTimeout,
		Poll:          pollInterval,
	}, nil
}

// Detach starts the update as a process of its own, with no stdio so the
// caller's run step sees both pipes close and ends, and returns at once.
func Detach(
	exe string,
	args ...string,
) error {
	_, err := launch(exe, args, detachAttempts())
	return err
}

func startDetached(
	exe string,
	args []string,
) (int, error) {
	return launch(exe, args, detachAttempts())
}

// launch starts exe with the first attributes that let it start: a detach
// that asks for more (leaving a job on Windows) may be refused where a plainer
// one is not.
func launch(
	exe string,
	args []string,
	attempts []*syscall.SysProcAttr,
) (int, error) {
	var errs []error
	for _, attrs := range attempts {
		cmd := exec.Command(exe, args...) // #nosec G204 G702 -- this binary's own path, or the daemon's own recorded command line
		cmd.SysProcAttr = attrs
		if err := cmd.Start(); err != nil {
			errs = append(errs, err)
			continue
		}
		pid := cmd.Process.Pid
		return pid, cmd.Process.Release()
	}
	return 0, fmt.Errorf("selfupdate: start %s: %w", exe, errors.Join(errs...))
}

// Run replaces the daemon's binary with newBin. The new binary is opened
// first: the file may be removed while the daemon is still shutting down.
//
// Nothing is swapped until the old daemon is gone. After that every failure
// puts the old binary back and starts it again, so the machine is never left
// without a daemon because an update did not take.
func (u *Updater) Run(
	ctx context.Context,
	newBin string,
) error {
	src, err := os.Open(newBin) // #nosec G304 -- the path the update's own fetch step wrote
	if err != nil {
		return fmt.Errorf("selfupdate: open new binary: %w", err)
	}
	defer src.Close() //nolint:errcheck // read only

	proc, err := u.Shutdown(ctx)
	if err != nil {
		return fmt.Errorf("selfupdate: no daemon answered on %s (self-update supports only a daemon on the "+
			"default local socket, not one started with a custom --host): %w", u.Socket, err)
	}
	slog.InfoContext(ctx, "selfupdate: daemon stopping", "pid", proc.PID)

	if err := u.waitGone(ctx, proc.PID); err != nil {
		return err
	}

	aside, err := u.swap(src)
	if err != nil {
		return errors.Join(err, u.restart(proc))
	}

	if err := u.startAndCheck(ctx, proc.Args); err != nil {
		return errors.Join(err, u.rollback(ctx, aside, proc))
	}

	_ = os.Remove(aside)
	slog.InfoContext(ctx, "selfupdate: new daemon healthy")
	return nil
}

func (u *Updater) waitGone(
	ctx context.Context,
	pid int,
) error {
	gone := func() bool { return !u.Alive(pid) && !u.Listening(ctx) }
	if !u.poll(ctx, u.StopTimeout, gone) {
		return fmt.Errorf("selfupdate: daemon %d still running after %s", pid, u.StopTimeout)
	}
	return nil
}

func (u *Updater) startAndCheck(
	ctx context.Context,
	args []string,
) error {
	pid, err := u.Start(u.Self, args)
	if err != nil {
		return err
	}
	if !u.poll(ctx, u.HealthTimeout, func() bool { return u.Healthy(ctx) }) {
		u.Kill(pid)
		u.poll(ctx, killTimeout, func() bool { return !u.Alive(pid) })
		return fmt.Errorf("selfupdate: new daemon %d not healthy after %s", pid, u.HealthTimeout)
	}
	u.recorded(pid)
	return nil
}

// poll reports whether cond became true before timeout.
func (u *Updater) poll(
	ctx context.Context,
	timeout time.Duration,
	cond func() bool,
) bool {
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) || ctx.Err() != nil {
			return false
		}
		time.Sleep(u.Poll)
	}
	return true
}

// swap moves the current binary aside and puts src in its place. The copy goes
// to a temporary name in the same directory first, so the final step is one
// rename and cannot leave a half-written binary at the self path. It returns
// the aside path, empty when there was nothing to move.
func (u *Updater) swap(
	src io.Reader,
) (string, error) {
	dir := filepath.Dir(u.Self)
	paths.SweepSelfAsides(dir)

	staged, err := stage(dir, src)
	if err != nil {
		return "", err
	}
	defer os.Remove(staged) //nolint:errcheck // already gone once it replaced the self path

	aside := ""
	if _, err := os.Stat(u.Self); err == nil {
		aside = freeAside(dir)
		if err := os.Rename(u.Self, aside); err != nil {
			return "", fmt.Errorf("selfupdate: move %s aside: %w", u.Self, err)
		}
	}

	if err := os.Rename(staged, u.Self); err != nil {
		return aside, errors.Join(fmt.Errorf("selfupdate: put new binary at %s: %w", u.Self, err), u.restore(aside))
	}
	return aside, nil
}

func stage(
	dir string,
	src io.Reader,
) (string, error) {
	tmp, err := os.CreateTemp(dir, "quiver.new-")
	if err != nil {
		return "", fmt.Errorf("selfupdate: stage new binary: %w", err)
	}
	_, copyErr := io.Copy(tmp, src)
	closeErr := tmp.Close()
	chmodErr := os.Chmod(tmp.Name(), 0o755) // #nosec G302 -- the staged file becomes the daemon's executable
	if err := errors.Join(copyErr, closeErr, chmodErr); err != nil {
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("selfupdate: stage new binary: %w", err)
	}
	return tmp.Name(), nil
}

func freeAside(
	dir string,
) string {
	for n := 1; ; n++ {
		aside := filepath.Join(dir, paths.SelfAsidePrefix+strconv.Itoa(n))
		if _, err := os.Lstat(aside); os.IsNotExist(err) {
			return aside
		}
	}
}

// restore puts the aside binary back at the self path.
func (u *Updater) restore(
	aside string,
) error {
	if aside == "" {
		return nil
	}
	_ = os.Remove(u.Self)
	if err := os.Rename(aside, u.Self); err != nil {
		return fmt.Errorf("selfupdate: restore %s: %w", u.Self, err)
	}
	return nil
}

func (u *Updater) rollback(
	ctx context.Context,
	aside string,
	proc dto.ShutdownDTO,
) error {
	slog.WarnContext(ctx, "selfupdate: rolling back to the previous binary")
	return errors.Join(u.restore(aside), u.restart(proc))
}

func (u *Updater) restart(
	proc dto.ShutdownDTO,
) error {
	pid, err := u.Start(proc.Exe, proc.Args)
	if err != nil {
		return err
	}
	u.recorded(pid)
	return nil
}

func (u *Updater) recorded(
	pid int,
) {
	if u.Started != nil {
		u.Started(pid)
	}
}
