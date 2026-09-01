package internal

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestContainer(t *testing.T) *Container {
	t.Helper()

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

func TestNew_UnusableHome_ReturnsEngineError(t *testing.T) {
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

	names := make([]string, 0, 4)
	for _, p := range c.shutdownPhases() {
		names = append(names, p.Name)
	}

	assert.Equal(t,
		[]string{"api shutdown", "app shutdown", "engine shutdown", "adapters close"},
		names,
		"stores must close only after every aggregate has drained")
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
