package selfupdate_test

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/cli/selfupdate"
)

const helperEnv = "QUIVER_SELFUPDATE_TEST_HELPER"

// TestHelperProcess is the child the real start, detach and kill paths launch:
// the test binary itself, so no external program is needed.
func TestHelperProcess(t *testing.T) {
	switch os.Getenv(helperEnv) {
	case "sleep":
		time.Sleep(time.Minute)
	case "exit":
	default:
		t.Skip("only runs as a child process")
	}
}

var helperArgs = []string{"-test.run=^TestHelperProcess$"}

// daemonOnSocket serves the endpoints the updater talks to on a unix socket,
// under a short path: socket paths are limited to about 100 bytes.
func daemonOnSocket(t *testing.T, healthy bool) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("unix socket")
	}
	dir, err := os.MkdirTemp("", "qsu")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "q.sock")

	mux := http.NewServeMux()
	mux.HandleFunc("/v0/health", func(w http.ResponseWriter, _ *http.Request) {
		if !healthy {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/v0/system/shutdown", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"pid":4242,"exe":"/q/quiver","args":["daemon"]}}`))
	})
	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(listener) }()
	t.Cleanup(func() { _ = srv.Close() })
	return socket
}

func newUpdater(t *testing.T, socket string) *selfupdate.Updater {
	t.Helper()
	u, err := selfupdate.New(socket, "/q/quiver")
	require.NoError(t, err)
	return u
}

func TestNew_TalksToTheDaemonOnTheSocket(t *testing.T) {
	socket := daemonOnSocket(t, true)
	u := newUpdater(t, socket)
	ctx := context.Background()

	proc, err := u.Shutdown(ctx)

	require.NoError(t, err)
	assert.Equal(t, 4242, proc.PID)
	assert.Equal(t, "/q/quiver", proc.Exe)
	assert.Equal(t, []string{"daemon"}, proc.Args)
	assert.True(t, u.Listening(ctx))
	assert.True(t, u.Healthy(ctx))
	assert.Equal(t, "/q/quiver", u.Self)
}

func TestNew_UnhealthyDaemonIsNotHealthy(t *testing.T) {
	u := newUpdater(t, daemonOnSocket(t, false))

	assert.False(t, u.Healthy(context.Background()))
}

func TestNew_NothingOnTheSocket(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "gone.sock")
	u := newUpdater(t, socket)
	ctx := context.Background()

	_, err := u.Shutdown(ctx)

	require.Error(t, err)
	assert.False(t, u.Listening(ctx))
	assert.False(t, u.Healthy(ctx))
}

func TestNew_AliveSeesTheLiveAndTheDead(t *testing.T) {
	u := newUpdater(t, "unused")
	assert.True(t, u.Alive(os.Getpid()))

	t.Setenv(helperEnv, "exit")
	cmd := exec.Command(os.Args[0], helperArgs...) // #nosec G204 -- the test binary itself
	require.NoError(t, cmd.Run())

	assert.False(t, u.Alive(cmd.Process.Pid))
}

func TestNew_KillEndsTheProcess(t *testing.T) {
	t.Setenv(helperEnv, "sleep")
	cmd := exec.Command(os.Args[0], helperArgs...) // #nosec G204 -- the test binary itself
	require.NoError(t, cmd.Start())
	u := newUpdater(t, "unused")

	u.Kill(cmd.Process.Pid)

	assert.Error(t, cmd.Wait(), "the process was killed, not allowed to finish")
}

func TestNew_StartLaunchesADetachedProcess(t *testing.T) {
	t.Setenv(helperEnv, "exit")
	u := newUpdater(t, "unused")

	pid, err := u.Start(os.Args[0], helperArgs)

	require.NoError(t, err)
	assert.Positive(t, pid)
}

func TestNew_StartReportsAMissingExecutable(t *testing.T) {
	u := newUpdater(t, "unused")

	_, err := u.Start(filepath.Join(t.TempDir(), "nope"), nil)

	require.Error(t, err)
}

func TestDetach(t *testing.T) {
	t.Setenv(helperEnv, "exit")

	require.NoError(t, selfupdate.Detach(os.Args[0], helperArgs...))
	require.Error(t, selfupdate.Detach(filepath.Join(t.TempDir(), "nope")))
}
