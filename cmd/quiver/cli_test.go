package main

import (
	"context"
	"net"
	"net/http"
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

func TestNewCLIDeps_Populated(t *testing.T) {
	deps := newCLIDeps()
	assert.Equal(t, version, deps.Version)
	assert.NotNil(t, deps.IsTTY)
	assert.NotNil(t, deps.EnsureDaemon)
	assert.False(t, deps.IsTTY(), "test stdout is not a terminal")
}

func TestNewCLIDeps_EnsureDaemonPropagatesManagerError(t *testing.T) {
	t.Setenv("HOME", "")
	deps := newCLIDeps()
	assert.Error(t, deps.EnsureDaemon(context.Background()))
}

func TestNewCLIDeps_EnsureDaemonSucceedsWhenLive(t *testing.T) {
	home := testutil.SocketDir(t)
	t.Setenv("HOME", home)
	socket := filepath.Join(home, ".quiver", "quiver.sock")
	require.NoError(t, os.MkdirAll(filepath.Dir(socket), 0o750))
	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	assert.NoError(t, newCLIDeps().EnsureDaemon(context.Background()))
}

func TestShouldManageDaemon(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		want bool
	}{
		{"no args", nil, true},
		{"regular command", []string{"list"}, true},
		{"daemon run", []string{"daemon"}, false},
		{"daemon with flag", []string{"daemon", "--host", "tcp://:1"}, false},
		{"flags before command", []string{"-o", "json", "ps"}, true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, shouldManageDaemon(tc.args))
		})
	}
}

// serveRuntimeSocket answers GET /v0/runtime on a Unix socket.
func serveRuntimeSocket(t *testing.T, socket, runtimesJSON string) {
	t.Helper()
	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)

	srv := &http.Server{
		ReadHeaderTimeout: time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"success":true,"data":` + runtimesJSON + `}`))
		}),
	}
	go func() { _ = srv.Serve(listener) }()
	t.Cleanup(func() { _ = srv.Close() })
}

// sleepProcess spawns a short-lived child whose PID can be signalled safely.
func sleepProcess(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	return cmd.Process.Pid
}

func newTestManager(t *testing.T) *daemon.Manager {
	t.Helper()
	dir := testutil.SocketDir(t)
	return &daemon.Manager{
		Socket:      filepath.Join(dir, "quiver.sock"),
		PIDFile:     filepath.Join(dir, "quiver.pid"),
		BootTimeout: time.Second,
	}
}

func TestStopIdleDaemon_NoPIDFileIsNoop(t *testing.T) {
	mgr := newTestManager(t)
	stopIdleDaemon(context.Background(), mgr)
}

func TestStopIdleDaemon_DeadSocketIsNoop(t *testing.T) {
	mgr := newTestManager(t)
	pid := sleepProcess(t)
	require.NoError(t, os.WriteFile(mgr.PIDFile, []byte(strconv.Itoa(pid)), 0o600))

	stopIdleDaemon(context.Background(), mgr)

	_, err := os.Stat(mgr.PIDFile)
	assert.NoError(t, err, "pid file must survive when the socket is dead")
}

func TestStopIdleDaemon_ActiveRuntimeKeepsDaemon(t *testing.T) {
	mgr := newTestManager(t)
	pid := sleepProcess(t)
	require.NoError(t, os.WriteFile(mgr.PIDFile, []byte(strconv.Itoa(pid)), 0o600))
	serveRuntimeSocket(t, mgr.Socket,
		`[{"namespace":"github.com/u/r","state":"running"}]`)

	stopIdleDaemon(context.Background(), mgr)

	_, err := os.Stat(mgr.PIDFile)
	assert.NoError(t, err, "active runtime must keep the daemon alive")
}

func TestStopIdleDaemon_ProbeFailureKeepsDaemon(t *testing.T) {
	mgr := newTestManager(t)
	pid := sleepProcess(t)
	require.NoError(t, os.WriteFile(mgr.PIDFile, []byte(strconv.Itoa(pid)), 0o600))

	listener, err := net.Listen("unix", mgr.Socket)
	require.NoError(t, err)
	srv := &http.Server{
		ReadHeaderTimeout: time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}),
	}
	go func() { _ = srv.Serve(listener) }()
	t.Cleanup(func() { _ = srv.Close() })

	stopIdleDaemon(context.Background(), mgr)

	_, statErr := os.Stat(mgr.PIDFile)
	assert.NoError(t, statErr, "unprobeable daemon must be left alone")
}

func TestStopIdleDaemon_IdleDaemonIsStopped(t *testing.T) {
	mgr := newTestManager(t)
	pid := sleepProcess(t)
	require.NoError(t, os.WriteFile(mgr.PIDFile, []byte(strconv.Itoa(pid)), 0o600))
	serveRuntimeSocket(t, mgr.Socket,
		`[{"namespace":"github.com/u/r","state":"ready"}]`)

	stopIdleDaemon(context.Background(), mgr)

	_, err := os.Stat(mgr.PIDFile)
	assert.True(t, os.IsNotExist(err), "idle daemon must be stopped and pid file removed")
}

func TestRootCommand_HasCLICommands(t *testing.T) {
	root := newRootCmd()
	for _, name := range []string{"install", "list", "ps", "context", "version"} {
		cmd, _, err := root.Find([]string{name})
		require.NoError(t, err, name)
		assert.Equal(t, name, cmd.Name())
	}
}
