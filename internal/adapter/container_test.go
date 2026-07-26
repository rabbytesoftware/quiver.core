package adapter

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
)

func TestNew_OpensSixHandlesAndClosesThemAll(t *testing.T) {
	c, err := New(WithHomeDir(t.TempDir()))
	require.NoError(t, err)

	require.NotNil(t, c.Arrow.Events)
	require.NotNil(t, c.Arrow.Snapshots)
	require.NotNil(t, c.Runtime.Events)
	require.NotNil(t, c.Runtime.Snapshots)
	require.NotNil(t, c.Quiver.Events)
	require.NotNil(t, c.Quiver.Snapshots)

	assert.NoError(t, c.Close())
}

func TestNew_CreatesSeparateSnapshotFiles(t *testing.T) {
	home := t.TempDir()
	c, err := New(WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	events, err := paths.EventsAt(home)
	require.NoError(t, err)
	for _, name := range []string{
		"arrow.db", "arrow_snapshots.db",
		"runtime.db", "runtime_snapshots.db",
		"collection.db", "collection_snapshots.db",
	} {
		assert.FileExists(t, filepath.Join(events, name))
	}
}

func TestNew_NoHomeDirOption_UsesProcessHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	c, err := New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	require.NotNil(t, c.Arrow.Events)
	require.NotNil(t, c.Arrow.Snapshots)
}

func TestNew_InvalidHomeDir_ReturnsError(t *testing.T) {
	_, err := New(WithHomeDir(string([]byte{0})))
	assert.Error(t, err)
}

func TestNew_ArrowEventStoreOpenFails_ReturnsError(t *testing.T) {
	home := t.TempDir()
	events, err := paths.EventsAt(home)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(events, "arrow.db"), 0o750))

	_, err = New(WithHomeDir(home))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "adapter: arrow event store:")
}

func TestNew_ArrowSnapshotStoreOpenFails_ReturnsError(t *testing.T) {
	home := t.TempDir()
	events, err := paths.EventsAt(home)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(events, "arrow_snapshots.db"), 0o750))

	_, err = New(WithHomeDir(home))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "adapter: arrow snapshot store:")
}

func TestNew_RuntimeStoreOpenFails_ClosesArrowStores(t *testing.T) {
	home := t.TempDir()
	events, err := paths.EventsAt(home)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(events, "runtime.db"), 0o750))

	_, err = New(WithHomeDir(home))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "adapter: runtime event store:")
}

func TestNew_QuiverStoreOpenFails_ClosesArrowAndRuntimeStores(t *testing.T) {
	home := t.TempDir()
	events, err := paths.EventsAt(home)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(events, "collection.db"), 0o750))

	_, err = New(WithHomeDir(home))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "adapter: quiver event store:")
}

func TestWithHomeDir_SetsOption(t *testing.T) {
	cfg := adapterOpts{}
	WithHomeDir("/tmp/quiver-test")(&cfg)
	assert.Equal(t, "/tmp/quiver-test", cfg.homeDir)
}

func TestResolveEventsPath_WithHomeDir_MatchesEventsAt(t *testing.T) {
	home := t.TempDir()
	got, err := resolveEventsPath(home)
	require.NoError(t, err)

	want, err := paths.EventsAt(home)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestOpenStores_Success_ReturnsStoresAndTwoClosers(t *testing.T) {
	events, err := paths.EventsAt(t.TempDir())
	require.NoError(t, err)

	stores, closers, err := openStores(events, "arrow", "arrow.db", "arrow_snapshots.db")
	require.NoError(t, err)
	require.NotNil(t, stores.Events)
	require.NotNil(t, stores.Snapshots)
	require.Len(t, closers, 2)

	closeAll(closers)
}

func TestOpenStores_EventStoreOpenFails_WrapsError(t *testing.T) {
	events, err := paths.EventsAt(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(events, "bad.db"), 0o750))

	_, closers, err := openStores(events, "arrow", "bad.db", "arrow_snapshots.db")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "adapter: arrow event store:")
	assert.Nil(t, closers)
}

func TestOpenStores_SnapshotStoreOpenFails_ClosesEventStore(t *testing.T) {
	events, err := paths.EventsAt(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(events, "bad_snapshots.db"), 0o750))

	_, closers, err := openStores(events, "arrow", "arrow.db", "bad_snapshots.db")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "adapter: arrow snapshot store:")
	assert.Nil(t, closers)
}

type spyCloser struct {
	closed bool
	err    error
}

func (s *spyCloser) Close() error {
	s.closed = true
	return s.err
}

func TestCloseAll_ClosesEveryCloserIgnoringErrors(t *testing.T) {
	a := &spyCloser{}
	b := &spyCloser{err: errors.New("close failed")}

	closeAll([]io.Closer{a, b})

	assert.True(t, a.closed)
	assert.True(t, b.closed)
}

func TestContainer_Close_ReturnsJoinedErrors(t *testing.T) {
	err1 := errors.New("close fail 1")
	err2 := errors.New("close fail 2")
	c := &Container{closers: []io.Closer{
		&spyCloser{err: err1},
		&spyCloser{},
		&spyCloser{err: err2},
	}}

	err := c.Close()
	require.Error(t, err)
	assert.ErrorIs(t, err, err1)
	assert.ErrorIs(t, err, err2)
}

func TestContainer_Close_NoClosers_ReturnsNil(t *testing.T) {
	c := &Container{}
	assert.NoError(t, c.Close())
}
