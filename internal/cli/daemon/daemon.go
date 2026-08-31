// Package daemon manages the local quiver.core process on behalf of the CLI:
// it probes the Unix socket, boots the daemon by re-executing the current
// binary when needed, and stops it via SIGTERM when nothing is active. The
// daemon's own graceful-shutdown chain (context cancellation on SIGTERM) does
// the actual teardown — this package only decides when to signal.
package daemon

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const probeTimeout = 250 * time.Millisecond

// BoundedBuffer is an io.Writer that keeps at most max bytes, discarding the
// rest. It is safe for concurrent Write and String because the child process
// writes while Ensure may read.
type BoundedBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

// NewBoundedBuffer creates a new bounded buffer with the given max capacity.
func NewBoundedBuffer(max int) *BoundedBuffer {
	return &BoundedBuffer{max: max}
}

func (b *BoundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if room := b.max - len(b.buf); room > 0 {
		if len(p) > room {
			p = p[:room]
		}
		b.buf = append(b.buf, p...)
	}

	return len(p), nil
}

func (b *BoundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return string(b.buf)
}

// Manager supervises the local daemon process.
type Manager struct {
	// Socket is the daemon's Unix socket path.
	Socket string
	// PIDFile records the booted daemon's PID; PIDFile+".lock" serialises
	// concurrent boot attempts.
	PIDFile string
	// Start launches the daemon and returns its PID.
	Start func() (int, error)
	// BootTimeout caps how long Ensure waits for the socket to come up.
	BootTimeout time.Duration
	// CaptureStderr returns whatever the booted daemon wrote to stderr.
	// It is set by startSelf and read by Ensure when the boot times out, so
	// a bind failure reaches the user instead of a bare timeout.
	CaptureStderr func() string
}

// NewManager builds a Manager with default paths under ~/.quiver and a Start
// that re-executes the current binary with the daemon subcommand.
func NewManager() (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("daemon: resolve home dir: %w", err)
	}
	dir := filepath.Join(home, ".quiver")

	m := &Manager{
		Socket:      filepath.Join(dir, "quiver.sock"),
		PIDFile:     filepath.Join(dir, "quiver.pid"),
		BootTimeout: 15 * time.Second,
	}
	m.Start = m.startSelf
	return m, nil
}

// startSelf forks the current binary as a detached daemon process.
func (m *Manager) startSelf() (int, error) {
	self, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("daemon: resolve executable: %w", err)
	}

	cmd := exec.Command(self, "daemon") // #nosec G204 -- argv is a literal; self is os.Executable()
	cmd.SysProcAttr = detachAttrs()
	cmd.Stdout = nil

	// A detached daemon writes nothing once it is healthy, so this buffer
	// only ever holds a startup failure. Bounded so a chatty daemon cannot
	// grow it without limit.
	buf := NewBoundedBuffer(4096)
	cmd.Stderr = buf
	m.CaptureStderr = buf.String

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("daemon: start process: %w", err)
	}

	pid := cmd.Process.Pid
	_ = cmd.Process.Release()

	return pid, nil
}

// bootFailure explains why the daemon is not answering. The daemon's own
// stderr is the useful answer when there is one; the timeout is only a
// fallback for a daemon that started and then hung.
func (m *Manager) bootFailure() string {
	if m.CaptureStderr != nil {
		if out := strings.TrimSpace(m.CaptureStderr()); out != "" {
			return "daemon failed to start: " + lastLine(out)
		}
	}

	return fmt.Sprintf("socket %s not live after %s", m.Socket, m.BootTimeout)
}

// lastLine returns the final non-empty line, which is where Go prints the
// error that ended the process.
func lastLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" {
			return t
		}
	}

	return s
}

// IsLive reports whether the daemon answers on its socket.
func (m *Manager) IsLive() bool {
	conn, err := net.DialTimeout("unix", m.Socket, probeTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// Ensure makes sure a daemon is serving on the socket, booting one if
// needed. Concurrent callers serialise on the boot lock: the loser waits for
// the winner's daemon instead of double-booting.
func (m *Manager) Ensure(ctx context.Context) error {
	if m.IsLive() {
		return nil
	}

	acquired, err := m.acquireBootLock()
	if err != nil {
		return err
	}
	if !acquired {
		return m.waitLive(ctx)
	}
	defer func() { _ = os.Remove(m.lockPath()) }()

	pid, err := m.Start()
	if err != nil {
		return fmt.Errorf("daemon: boot: %w", err)
	}
	if err := os.WriteFile(m.PIDFile, []byte(strconv.Itoa(pid)), 0o600); err != nil {
		return fmt.Errorf("daemon: write pid file: %w", err)
	}

	return m.waitLive(ctx)
}

func (m *Manager) lockPath() string { return m.PIDFile + ".lock" }

// acquireBootLock returns true when this process should boot the daemon.
// A lock older than BootTimeout is treated as a crashed boot and reclaimed.
func (m *Manager) acquireBootLock() (bool, error) {
	if err := os.MkdirAll(filepath.Dir(m.PIDFile), 0o750); err != nil {
		return false, fmt.Errorf("daemon: create state dir: %w", err)
	}

	f, err := os.OpenFile(m.lockPath(), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err == nil {
		_, _ = fmt.Fprintf(f, "%d", os.Getpid())
		_ = f.Close()
		return true, nil
	}
	if !os.IsExist(err) {
		return false, fmt.Errorf("daemon: acquire boot lock: %w", err)
	}

	info, statErr := os.Stat(m.lockPath())
	if statErr == nil && time.Since(info.ModTime()) > m.BootTimeout {
		_ = os.Remove(m.lockPath())
		return m.acquireBootLock()
	}
	return false, nil
}

// waitLive polls the socket until it answers, ctx is cancelled, or
// BootTimeout elapses.
func (m *Manager) waitLive(ctx context.Context) error {
	deadline := time.Now().Add(m.BootTimeout)
	for {
		if m.IsLive() {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("daemon: %s", m.bootFailure())
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("daemon: waiting for socket: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// ReadPID returns the PID recorded at boot time.
func (m *Manager) ReadPID() (int, error) {
	raw, err := os.ReadFile(m.PIDFile)
	if err != nil {
		return 0, fmt.Errorf("daemon: read pid file: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		return 0, fmt.Errorf("daemon: parse pid file: %w", err)
	}
	return pid, nil
}

// Stop sends SIGTERM to the recorded daemon process and removes the PID
// file. The daemon drains and exits on its own; Stop does not wait. A missing
// PID file is a no-op.
func (m *Manager) Stop() error {
	pid, err := m.ReadPID()
	if os.IsNotExist(unwrapPathError(err)) {
		return nil
	}
	if err != nil {
		return err
	}

	proc, err := os.FindProcess(pid)
	if err == nil {
		_ = proc.Signal(syscall.SIGTERM)
	}
	if err := os.Remove(m.PIDFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("daemon: remove pid file: %w", err)
	}
	return nil
}

func unwrapPathError(err error) error {
	for err != nil {
		if pe, ok := err.(*os.PathError); ok { //nolint:errorlint // walking the chain manually
			return pe
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return err
		}
		err = u.Unwrap()
	}
	return err
}
