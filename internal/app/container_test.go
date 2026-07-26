package app_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/adapter"
	"github.com/rabbytesoftware/quiver.core/internal/app"
	"github.com/rabbytesoftware/quiver.core/internal/engine"
)

// newContainer wires the real thing on a temp home: the container's job is to
// compose engines and adapters, and a stubbed engine would not exercise that.
func newContainer(t *testing.T) *app.Container {
	t.Helper()

	home := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	engines, err := engine.New(ctx, engine.WithHomeDir(home))
	require.NoError(t, err)

	adapters, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := app.New(engines, adapters, app.WithHomeDir(home))
	require.NoError(t, err)
	return c
}

func TestNew_WiresEveryUsecase(t *testing.T) {
	c := newContainer(t)

	assert.NotNil(t, c.Arrow)
	assert.NotNil(t, c.Runtime)
	assert.NotNil(t, c.Collection)
	assert.NotNil(t, c.Search)
	assert.NotNil(t, c.Hub)
}

// TestNew_BuildsDiscoveryWhenVaultAndManifoldExist: a real engine container has
// both, so the discovery usecase must be there. It is nil only for a container
// built without them, which the repository layer already refuses to half-build.
func TestNew_BuildsDiscoveryWhenVaultAndManifoldExist(t *testing.T) {
	assert.NotNil(t, newContainer(t).Discovery)
}

func TestNew_MissingAdapters_ReturnsError(t *testing.T) {
	home := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	engines, err := engine.New(ctx, engine.WithHomeDir(home))
	require.NoError(t, err)

	_, err = app.New(engines, &adapter.Container{}, app.WithHomeDir(home))
	require.Error(t, err)
}

// TestNew_UnusableHome_ReturnsError points the home at a regular file, so the
// store directory cannot be created and the container fails rather than
// starting half-built.
func TestNew_UnusableHome_ReturnsError(t *testing.T) {
	home := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	engines, err := engine.New(ctx, engine.WithHomeDir(home))
	require.NoError(t, err)

	adapters, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	notADir := filepath.Join(home, "file")
	require.NoError(t, os.WriteFile(notADir, []byte("x"), 0o600))

	_, err = app.New(engines, adapters, app.WithHomeDir(notADir))
	require.Error(t, err)
}

func TestContainer_StartAndShutdown(t *testing.T) {
	c := newContainer(t)

	ctx := context.Background()
	c.Start(ctx)

	require.NoError(t, c.Shutdown(ctx))
}
