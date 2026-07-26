package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

func TestNew_Success_PopulatesContainer(t *testing.T) {
	c, err := New(context.Background(), WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = c.netbridgeEvents.Close()
		_ = c.netbridgeSnapshots.Close()
	})

	assert.NotNil(t, c.Vault)
	assert.NotNil(t, c.Manifold)
	assert.NotNil(t, c.Wizard)
	assert.NotNil(t, c.Netbridge)
	assert.NotNil(t, c.DepTree)
}

func TestNew_Success_CreatesNetbridgeDBFiles(t *testing.T) {
	home := t.TempDir()

	c, err := New(context.Background(), WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = c.netbridgeEvents.Close()
		_ = c.netbridgeSnapshots.Close()
	})

	events, err := paths.EventsAt(home)
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(events, "netbridge.db"))
	assert.FileExists(t, filepath.Join(events, "netbridge_snapshots.db"))
}

func TestNew_NetbridgeEventStoreOpenFails_ReturnsError(t *testing.T) {
	home := t.TempDir()
	events, err := paths.EventsAt(home)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(events, "netbridge.db"), 0o750))

	_, err = New(context.Background(), WithHomeDir(home))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "engine container:")
	assert.Contains(t, err.Error(), "eventstore:")
}

func TestNew_NetbridgeSnapshotStoreOpenFails_ReturnsError(t *testing.T) {
	home := t.TempDir()
	events, err := paths.EventsAt(home)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(events, "netbridge_snapshots.db"), 0o750))

	_, err = New(context.Background(), WithHomeDir(home))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "engine container:")
	assert.Contains(t, err.Error(), "snapshotstore:")
}

func TestNew_InvalidHomeDir_ReturnsError(t *testing.T) {
	_, err := New(context.Background(), WithHomeDir(string([]byte{0})))
	assert.Error(t, err)
}

func TestNew_NoHomeDirOption_UsesProcessHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	c, err := New(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = c.netbridgeEvents.Close()
		_ = c.netbridgeSnapshots.Close()
	})

	assert.NotNil(t, c.Netbridge)
}

func TestContainer_Start_StartsVaultWithoutBlocking(t *testing.T) {
	c, err := New(context.Background(), WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = c.netbridgeEvents.Close()
		_ = c.netbridgeSnapshots.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.Start(ctx)
}

func TestCloseAll_ReleasesEveryHandle(t *testing.T) {
	first := &countingCloser{}
	second := &countingCloser{err: errors.New("close boom")}
	third := &countingCloser{}

	closeAll(first, second, third)

	assert.Equal(t, 1, first.calls)
	assert.Equal(t, 1, second.calls)
	assert.Equal(t, 1, third.calls, "a failed close must not skip the remaining handles")
}

func TestContainer_Shutdown_DrainsNetbridgeThenClosesHandles(t *testing.T) {
	c, err := New(context.Background(), WithHomeDir(t.TempDir()))
	require.NoError(t, err)

	require.NoError(t, c.Shutdown(context.Background()))

	assert.Error(t, c.Shutdown(context.Background()),
		"netbridge must report its aggregate is already drained on a second call")
}

func TestContainer_Shutdown_RunsEveryPhaseDespiteFailure(t *testing.T) {
	drainErr := errors.New("drain boom")
	eventsErr := errors.New("events boom")
	snapshotsErr := errors.New("snapshots boom")

	c := &Container{
		Netbridge:          &mocks.Netbridge{ShutdownErr: drainErr},
		netbridgeEvents:    &countingCloser{err: eventsErr},
		netbridgeSnapshots: &countingCloser{err: snapshotsErr},
	}

	err := c.Shutdown(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, drainErr)
	assert.ErrorIs(t, err, eventsErr, "a failed drain must not skip closing the event store")
	assert.ErrorIs(t, err, snapshotsErr, "a failed close must not skip the remaining handles")
}

type countingCloser struct {
	err   error
	calls int
}

func (c *countingCloser) Close() error {
	c.calls++
	return c.err
}
