package daemon_test

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/daemon"
	"github.com/rabbytesoftware/quiver.core/internal/cli/testutil"
)

func listenUnix(t *testing.T, socket string) net.Listener {
	t.Helper()
	ln, err := net.Listen("unix", socket)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	return ln
}

func newManager(t *testing.T, start func() (int, error)) (*daemon.Manager, string) {
	t.Helper()
	dir := testutil.SocketDir(t)
	socket := filepath.Join(dir, "quiver.sock")
	m := &daemon.Manager{
		Socket:      socket,
		PIDFile:     filepath.Join(dir, "quiver.pid"),
		Start:       start,
		BootTimeout: 2 * time.Second,
	}
	return m, socket
}

// ─── IsLive ──────────────────────────────────────────────────────────────────

func TestIsLive_FalseWhenNoSocket(t *testing.T) {
	m, _ := newManager(t, nil)
	assert.False(t, m.IsLive())
}

func TestIsLive_TrueWhenListening(t *testing.T) {
	m, socket := newManager(t, nil)
	listenUnix(t, socket)
	assert.True(t, m.IsLive())
}

func TestIsLive_FalseWhenSocketFileIsStale(t *testing.T) {
	m, socket := newManager(t, nil)
	ln := listenUnix(t, socket)
	_ = ln.Close()
	assert.False(t, m.IsLive())
}

// ─── Ensure ──────────────────────────────────────────────────────────────────

func TestEnsure_NoopWhenAlreadyLive(t *testing.T) {
	started := false
	m, socket := newManager(t, func() (int, error) { started = true; return 0, nil })
	listenUnix(t, socket)

	require.NoError(t, m.Ensure(context.Background()))
	assert.False(t, started, "Start must not run when the socket is live")
}

func TestEnsure_BootsAndWaitsForSocket(t *testing.T) {
	var m *daemon.Manager
	var socket string
	m, socket = newManager(t, func() (int, error) {
		go func() {
			time.Sleep(100 * time.Millisecond)
			ln, err := net.Listen("unix", socket)
			if err != nil {
				return
			}
			for {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				_ = conn.Close()
			}
		}()
		return 4242, nil
	})

	require.NoError(t, m.Ensure(context.Background()))
	assert.True(t, m.IsLive())

	pid, err := m.ReadPID()
	require.NoError(t, err)
	assert.Equal(t, 4242, pid)
}

func TestEnsure_BootFailureIncludesDaemonStderr(t *testing.T) {
	testutil.RequireUnix(t)

	m, _ := newManager(t, func() (int, error) { return 0, nil })
	m.BootTimeout = 300 * time.Millisecond
	m.CaptureStderr = func() string { return "bind: invalid argument" }

	err := m.Ensure(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "bind: invalid argument",
		"the daemon's own failure must reach the user, not just a timeout")
}

func TestEnsure_TimesOutWhenDaemonNeverListens(t *testing.T) {
	m, _ := newManager(t, func() (int, error) { return 1, nil })
	m.BootTimeout = 300 * time.Millisecond

	err := m.Ensure(context.Background())
	assert.Error(t, err)
}

func TestEnsure_StartFailureErrors(t *testing.T) {
	m, _ := newManager(t, func() (int, error) { return 0, os.ErrPermission })

	err := m.Ensure(context.Background())
	assert.Error(t, err)
}

func TestEnsure_ContextCancelAborts(t *testing.T) {
	m, _ := newManager(t, func() (int, error) { return 1, nil })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := m.Ensure(ctx)
	assert.Error(t, err)
}

func TestEnsure_SecondCallerWaitsForBootInProgress(t *testing.T) {
	m, socket := newManager(t, func() (int, error) {
		t.Fatal("Start must not run while another boot holds the lock")
		return 0, nil
	})

	// Simulate another CLI holding the boot lock, then bringing up the socket.
	require.NoError(t, os.WriteFile(m.PIDFile+".lock", []byte("1"), 0o600))
	go func() {
		time.Sleep(150 * time.Millisecond)
		ln, err := net.Listen("unix", socket)
		if err != nil {
			return
		}
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	require.NoError(t, m.Ensure(context.Background()))
	assert.True(t, m.IsLive())
}

func TestEnsure_StaleLockIsReclaimed(t *testing.T) {
	booted := false
	var m *daemon.Manager
	var socket string
	m, socket = newManager(t, func() (int, error) {
		booted = true
		listenUnix(t, socket)
		return 99, nil
	})
	m.BootTimeout = 400 * time.Millisecond

	// A stale lock from a crashed CLI, older than the boot timeout.
	require.NoError(t, os.WriteFile(m.PIDFile+".lock", []byte("1"), 0o600))
	old := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(m.PIDFile+".lock", old, old))

	require.NoError(t, m.Ensure(context.Background()))
	assert.True(t, booted)
}

// ─── Stop ────────────────────────────────────────────────────────────────────

func TestStop_SignalsRecordedPID(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	m, _ := newManager(t, nil)
	require.NoError(t, os.WriteFile(m.PIDFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0o600))

	require.NoError(t, m.Stop())

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done: // process terminated by SIGTERM
	case <-time.After(2 * time.Second):
		t.Fatal("process was not terminated")
	}

	_, err := os.Stat(m.PIDFile)
	assert.True(t, os.IsNotExist(err), "pid file should be removed after stop")
}

func TestStop_NoPIDFileIsNoop(t *testing.T) {
	m, _ := newManager(t, nil)
	assert.NoError(t, m.Stop())
}

func TestStop_GarbagePIDFileErrors(t *testing.T) {
	m, _ := newManager(t, nil)
	require.NoError(t, os.WriteFile(m.PIDFile, []byte("not-a-pid"), 0o600))
	assert.Error(t, m.Stop())
}

func TestReadPID_MissingFileErrors(t *testing.T) {
	m, _ := newManager(t, nil)
	_, err := m.ReadPID()
	assert.Error(t, err)
}

// ─── defaults ────────────────────────────────────────────────────────────────

func TestNewManager_DefaultPaths(t *testing.T) {
	m, err := daemon.NewManager()
	require.NoError(t, err)
	assert.Contains(t, m.Socket, "quiver.sock")
	assert.Contains(t, m.PIDFile, "quiver.pid")
	assert.NotNil(t, m.Start)
	assert.Greater(t, m.BootTimeout, time.Second)
}

// ─── boundedBuffer ───────────────────────────────────────────────────────────

func TestBoundedBuffer_TruncatesAtMax(t *testing.T) {
	buf := daemon.NewBoundedBuffer(10)
	n, err := buf.Write([]byte("hello world"))
	require.NoError(t, err)
	assert.Equal(t, 10, n, "Write should return bytes actually written")
	assert.Equal(t, "hello worl", buf.String(), "buffer should truncate at max")
}

func TestBoundedBuffer_SafeToWriteAfterCapacityReached(t *testing.T) {
	buf := daemon.NewBoundedBuffer(5)
	_, _ = buf.Write([]byte("hello"))
	n, err := buf.Write([]byte(" world"))
	require.NoError(t, err)
	assert.Equal(t, 6, n, "Write should return input length even if buffer is full")
	assert.Equal(t, "hello", buf.String(), "buffer should ignore writes after capacity")
}

func TestBoundedBuffer_StringReturnsWritten(t *testing.T) {
	buf := daemon.NewBoundedBuffer(100)
	_, _ = buf.Write([]byte("test output"))
	assert.Equal(t, "test output", buf.String())
}

func TestBoundedBuffer_PartialWrite(t *testing.T) {
	buf := daemon.NewBoundedBuffer(8)
	_, _ = buf.Write([]byte("hello"))
	n, err := buf.Write([]byte("world"))
	require.NoError(t, err)
	assert.Equal(t, 3, n, "Write should return bytes actually written")
	assert.Equal(t, "hellowor", buf.String(), "only 3 bytes fit in remaining space")
}

func TestBoundedBuffer_EmptyString(t *testing.T) {
	buf := daemon.NewBoundedBuffer(10)
	assert.Equal(t, "", buf.String())
}

// ─── coverage: remaining branches ────────────────────────────────────────────

func TestMain(m *testing.M) {
	// When startSelf re-executes this test binary with the "daemon" arg,
	// behave as a short-lived fake daemon instead of re-running the suite.
	if os.Getenv("QUIVER_TEST_DAEMON_HELPER") == "1" &&
		len(os.Args) > 1 && os.Args[len(os.Args)-1] == "daemon" {
		time.Sleep(50 * time.Millisecond)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestNewManager_NoHomeErrors(t *testing.T) {
	testutil.RequireUnix(t)

	t.Setenv("HOME", "")
	_, err := daemon.NewManager()
	assert.Error(t, err)
}

func TestStartSelf_ForksDetachedProcess(t *testing.T) {
	t.Setenv("QUIVER_TEST_DAEMON_HELPER", "1")
	m, err := daemon.NewManager()
	require.NoError(t, err)

	pid, err := m.Start()
	require.NoError(t, err)
	assert.Greater(t, pid, 0)
}

func TestEnsure_LockDirUnwritableErrors(t *testing.T) {
	dir := t.TempDir()
	readonly := filepath.Join(dir, "ro")
	require.NoError(t, os.MkdirAll(readonly, 0o500))
	t.Cleanup(func() { _ = os.Chmod(readonly, 0o700) })

	m := &daemon.Manager{
		Socket:      filepath.Join(dir, "quiver.sock"),
		PIDFile:     filepath.Join(readonly, "sub", "quiver.pid"),
		Start:       func() (int, error) { return 1, nil },
		BootTimeout: 300 * time.Millisecond,
	}
	assert.Error(t, m.Ensure(context.Background()))
}

func TestEnsure_LockPathIsDirectoryErrors(t *testing.T) {
	m, _ := newManager(t, func() (int, error) { return 1, nil })
	require.NoError(t, os.MkdirAll(m.PIDFile+".lock", 0o750))

	assert.Error(t, m.Ensure(context.Background()))
}

func TestStop_UnremovablePIDFileErrors(t *testing.T) {
	testutil.RequireUnprivileged(t)

	dir := testutil.SocketDir(t)
	stateDir := filepath.Join(dir, "state")
	require.NoError(t, os.MkdirAll(stateDir, 0o700))
	m := &daemon.Manager{
		Socket:  filepath.Join(dir, "quiver.sock"),
		PIDFile: filepath.Join(stateDir, "quiver.pid"),
	}
	cmd := exec.Command("sleep", "60")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	require.NoError(t, os.WriteFile(m.PIDFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0o600))
	require.NoError(t, os.Chmod(stateDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(stateDir, 0o700) })

	assert.Error(t, m.Stop())
}
