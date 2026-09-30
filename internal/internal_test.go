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

func TestWithGateway_SetsOption(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	cfg := internalOpts{}
	WithGateway(ln, "tcp")(&cfg)

	assert.Same(t, ln, cfg.listener)
	assert.Equal(t, "tcp", cfg.scheme)
}

func TestPrepareGateway_Unix_BindsAndReturnsScheme(t *testing.T) {
	f, err := os.CreateTemp("", "qv-internal-prepare-*.sock")
	require.NoError(t, err)
	sockPath := f.Name()
	require.NoError(t, f.Close())
	require.NoError(t, os.Remove(sockPath))
	t.Cleanup(func() { _ = os.Remove(sockPath) })

	ln, scheme, err := PrepareGateway("unix://" + sockPath)
	require.NoError(t, err)
	defer ln.Close()

	assert.Equal(t, "unix", scheme)

	conn, err := net.Dial("unix", sockPath)
	require.NoError(t, err, "PrepareGateway must return a listener that is actually bound")
	conn.Close()
}

func TestPrepareGateway_TCP_BindsAndReturnsScheme(t *testing.T) {
	ln, scheme, err := PrepareGateway("tcp://127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	assert.Equal(t, "tcp", scheme)

	conn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	conn.Close()
}

func TestPrepareGateway_MalformedHost_ReturnsGatewayError(t *testing.T) {
	_, _, err := PrepareGateway("not-a-uri")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internal: gateway")
}

// The regression this whole option exists for: a daemon that loses the race
// for its socket path must fail via PrepareGateway alone, before New -- and
// so before adapter.New's sqlite migration -- is ever reached. See
// cmd/quiver's daemon race test for the full end-to-end reproduction; this
// pins the seam PrepareGateway itself is responsible for.
func TestPrepareGateway_DaemonAlreadyRunning_ReturnsErrorWithoutBindingASecondListener(t *testing.T) {
	f, err := os.CreateTemp("", "qv-internal-prepare-*.sock")
	require.NoError(t, err)
	sockPath := f.Name()
	require.NoError(t, f.Close())
	require.NoError(t, os.Remove(sockPath))
	t.Cleanup(func() { _ = os.Remove(sockPath) })

	winner, _, err := PrepareGateway("unix://" + sockPath)
	require.NoError(t, err)
	defer winner.Close()

	_, _, err = PrepareGateway("unix://" + sockPath)
	require.Error(t, err, "a second PrepareGateway for the same path must fail rather than hand back a listener nothing else can serve on")
	assert.Contains(t, err.Error(), "internal: gateway")
}

// TestNew_WithGateway_StartReusesTheProvidedListener proves the seam
// PrepareGateway exists for: once WithGateway hands New a listener, Start
// must use it as-is, never re-resolving host/config or asking gateway.New for
// a second one of its own. A host argument that would fail Start's own
// fallback resolution (bindGateway) is passed on purpose -- Start reaching
// that fallback at all would surface as this test failing instead of the
// silent double-bind the fallback exists to prevent in the first place.
func TestNew_WithGateway_StartReusesTheProvidedListener(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	ln, scheme, err := PrepareGateway("tcp://127.0.0.1:0")
	require.NoError(t, err)

	c, err := New(
		context.Background(),
		"v0.0.0-test",
		"test-build",
		WithHomeDir(t.TempDir()),
		WithGateway(ln, scheme),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Shutdown() })

	assert.True(t, c.authGate.Required(), "New must flip authGate itself for a pre-bound tcp listener, since Start will skip its own scheme resolution")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.NoError(t, c.Start(ctx, "not-a-uri"), "Start must reuse the pre-bound listener rather than resolving host at all")
}

func TestWithRecommendations_SetsOption(t *testing.T) {
	cfg := internalOpts{}

	WithRecommendations()(&cfg)

	assert.True(t, cfg.recommendations)
}

func TestNew_WithoutRecommendations_DoesNotLaunchTheRefreshLoop(t *testing.T) {
	c := newTestContainer(t)
	t.Cleanup(func() { _ = c.Shutdown() })

	assert.False(t, c.recommendations, "a container built without the option never reaches a git host")
}

// The context is already cancelled, so the loop the option launches stops before
// it can search anything: this covers the wiring without a network call.
func TestContainer_Start_WithRecommendations_LaunchesTheLoopAndShutsDownCleanly(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	c, err := New(context.Background(), "v0.0.0-test", "test-build", WithHomeDir(t.TempDir()), WithRecommendations())
	require.NoError(t, err)
	require.True(t, c.recommendations)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.NoError(t, c.Start(ctx, "tcp://127.0.0.1:0"))
}
