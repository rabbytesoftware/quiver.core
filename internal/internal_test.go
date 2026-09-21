package internal

import (
	"context"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/selfupdate"
)

func newTestContainer(t *testing.T) *Container {
	t.Helper()

	// New now wires core.NewAt into construction, which reassigns
	// slog.Default() for the process. Restore it so one test's logger never
	// leaks into the next.
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	c, err := New(context.Background(), "v0.0.0-test", "test-build", WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	return c
}

func TestWithHomeDir_SetsOption(t *testing.T) {
	cfg := internalOpts{}
	WithHomeDir("/tmp/quiver-test")(&cfg)
	assert.Equal(t, "/tmp/quiver-test", cfg.homeDir)
}

func TestNew_Success_WiresEveryLayer(t *testing.T) {
	c := newTestContainer(t)
	t.Cleanup(func() { _ = c.Shutdown() })

	assert.NotNil(t, c.Engines)
	assert.NotNil(t, c.Adapters)
	assert.NotNil(t, c.App)
	assert.NotNil(t, c.API)
}

func TestNew_WithHomeDir_ScopesConfigAndLoggingToo(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	home := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(home, "config.yaml"), []byte(`config:
  logger:
    enabled: true
    level: info
`), 0o644))

	c, err := New(context.Background(), "v0.0.0-test", "test-build", WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Shutdown() })

	// core.NewAt reads config.yaml and initializes logging from the same
	// home this container was given — a regression here would mean it
	// silently fell back to the real process ~/.quiver instead. lumberjack
	// creates the log file lazily on first write, so force one.
	slog.Info("probe")
	_, statErr := os.Stat(filepath.Join(home, "logs", "Quiver.log"))
	assert.NoError(t, statErr, "expected Quiver.log under the container's own home, not ~/.quiver")
}

func TestNew_UnusableHome_ReturnsEngineError(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	// A regular file where a directory must be created makes os.MkdirAll fail
	// with ENOTDIR from its portable fast path, so this is unusable on every
	// platform. Sabotaging HOME instead would be a no-op on Windows, which
	// resolves the home directory from USERPROFILE.
	home := filepath.Join(t.TempDir(), "home")
	require.NoError(t, os.WriteFile(home, []byte("not a directory"), 0o600))

	_, err := New(context.Background(), "v0.0.0-test", "test-build", WithHomeDir(home))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal: engine")
}

func TestContainer_ShutdownPhases_ClosesStoresAfterEveryDrain(t *testing.T) {
	c := newTestContainer(t)
	t.Cleanup(func() { _ = c.Shutdown() })

	names := make([]string, 0, 5)
	for _, p := range c.shutdownPhases() {
		names = append(names, p.Name)
	}

	assert.Equal(t,
		[]string{"api shutdown", "app shutdown", "engine shutdown", "adapters close", "logger close"},
		names,
		"stores must close only after every aggregate has drained, logger last of all")
}

func TestContainer_Shutdown_DrainsAndClosesEveryLayer(t *testing.T) {
	c := newTestContainer(t)

	require.NoError(t, c.Shutdown())
}

func TestContainer_Shutdown_RunsEveryPhaseDespiteFailure(t *testing.T) {
	c := newTestContainer(t)

	require.NoError(t, c.Shutdown())

	// A second pass fails on every already-drained aggregate. Both the app and
	// the engine phase must appear: the engine phase runs only if the app
	// phase's failure did not abort the sequence.
	err := c.Shutdown()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal: app shutdown")
	assert.Contains(t, err.Error(), "internal: engine shutdown")
}

func TestContainer_Start_ShutdownFails_ReturnsError(t *testing.T) {
	c := newTestContainer(t)
	require.NoError(t, c.Shutdown())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.Start(ctx, "tcp://127.0.0.1:0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal: app shutdown")
}

func TestContainer_Start_ContextCancelled_ShutsDownCleanly(t *testing.T) {
	c := newTestContainer(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.NoError(t, c.Start(ctx, "tcp://127.0.0.1:0"))
}

func TestContainer_Start_ListenerFails_ReturnsGatewayError(t *testing.T) {
	c := newTestContainer(t)
	t.Cleanup(func() { _ = c.Shutdown() })

	err := c.Start(context.Background(), "grpc://127.0.0.1:0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal: gateway")
}

func TestContainer_Start_MalformedHost_ReturnsGatewayError(t *testing.T) {
	c := newTestContainer(t)
	t.Cleanup(func() { _ = c.Shutdown() })

	// Missing "://" entirely: gateway.Scheme itself rejects this, before
	// gateway.New (and thus the listener) is ever reached.
	err := c.Start(context.Background(), "not-a-uri")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal: gateway")
}

func TestContainer_Start_TCPHost_RequiresAuth(t *testing.T) {
	c := newTestContainer(t)
	t.Cleanup(func() { _ = c.Shutdown() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.NoError(t, c.Start(ctx, "tcp://127.0.0.1:0"))
	assert.True(t, c.authGate.Required())
}

func TestContainer_Start_UnixHost_DoesNotRequireAuth(t *testing.T) {
	c := newTestContainer(t)
	t.Cleanup(func() { _ = c.Shutdown() })

	f, err := os.CreateTemp("", "qv-internal-*.sock")
	require.NoError(t, err)
	sockPath := f.Name()
	require.NoError(t, f.Close())
	require.NoError(t, os.Remove(sockPath))
	t.Cleanup(func() { _ = os.Remove(sockPath) })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.NoError(t, c.Start(ctx, "unix://"+sockPath))
	assert.False(t, c.authGate.Required())
}

func TestWithSelfUpdateTrigger_SetsOption(t *testing.T) {
	trig := selfupdate.NewTrigger(nil)

	cfg := internalOpts{}
	WithSelfUpdateTrigger(trig)(&cfg)

	assert.Same(t, trig, cfg.selfUpdateTrigger)
}

// A daemon built with a trigger has to reach Start the same as one built
// without: the succession is wired in beside every other callback, not instead
// of the container.
func TestNew_WithSelfUpdateTrigger_StillWiresEveryLayer(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	c, err := New(
		context.Background(),
		"v0.0.0-test",
		"test-build",
		WithHomeDir(t.TempDir()),
		WithSelfUpdateTrigger(selfupdate.NewTrigger(nil)),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Shutdown() })

	assert.NotNil(t, c.App)
	assert.NotNil(t, c.API)
}

// The whole point of threading the trigger into the container is that firing
// it ends a running daemon. A trigger that only recorded the handover would
// leave Start blocked on its context until a signal that is never coming, and
// the relaunch after Start would never be reached.
func TestContainer_Start_TriggerFired_ShutsDownTheRunningDaemon(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	trigger := selfupdate.NewTrigger(cancel)

	c, err := New(
		context.Background(),
		"v0.0.0-test",
		"test-build",
		WithHomeDir(t.TempDir()),
		WithSelfUpdateTrigger(trigger),
	)
	require.NoError(t, err)

	f, err := os.CreateTemp("", "qv-selfupdate-*.sock")
	require.NoError(t, err)
	sockPath := f.Name()
	require.NoError(t, f.Close())
	require.NoError(t, os.Remove(sockPath))
	t.Cleanup(func() { _ = os.Remove(sockPath) })

	done := make(chan error, 1)
	go func() { done <- c.Start(ctx, "unix://"+sockPath) }()

	// Fire only once the daemon is really serving, so this exercises the
	// shutdown of a running daemon rather than the cancellation of one still
	// booting.
	require.Eventually(t, func() bool {
		conn, dialErr := net.Dial("unix", sockPath)
		if dialErr != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 30*time.Second, 20*time.Millisecond, "the daemon never started serving")

	trigger.Fire("/vault/quiver-core/v2/quiver-new")

	select {
	case startErr := <-done:
		require.NoError(t, startErr)
	case <-time.After(30 * time.Second):
		t.Fatal("firing the trigger did not bring the daemon down")
	}

	assert.True(t, trigger.Fired())
	assert.Equal(t, "/vault/quiver-core/v2/quiver-new", trigger.NewBinaryPath())
}
