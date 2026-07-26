package app

import (
	"context"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/adapter"
	"github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine"
)

// testAddArrowCmd is a minimal Command[domain.Arrow] used to drive newAsynx's
// wiring without reaching into the arrow repository's internal command package,
// which internal/app is not allowed to import.
type testAddArrowCmd struct {
	ns domain.Namespace
}

func (c testAddArrowCmd) AggregateID() string {
	return c.ns.String()
}

func (c testAddArrowCmd) EventName() string {
	return "arrow.added." + c.ns.String()
}

func (c testAddArrowCmd) ShouldSnapshot() bool {
	return true
}

func (c testAddArrowCmd) Validate(_ *domain.Arrow) error {
	return nil
}

func (c testAddArrowCmd) EmitEvent(_ *domain.Arrow) domain.Arrow {
	return domain.Arrow{Namespace: c.ns}
}

func newTestStores(t *testing.T) adapter.Stores {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)
	return adapter.Stores{Events: es, Snapshots: ss}
}

func TestNewAsynx_Success_BuildsUsableInstance(t *testing.T) {
	ax, err := newAsynx[domain.Arrow](newTestStores(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	ns := domain.Namespace("github.com/user/repo@v1")
	_, err = ax.Send(context.Background(), testAddArrowCmd{ns: ns})
	require.NoError(t, err)

	got, err := ax.Get(context.Background(), ns.String())
	require.NoError(t, err)
	assert.Equal(t, ns, got.Namespace)
}

func TestNewAsynx_MissingEventStore_ReturnsError(t *testing.T) {
	stores := newTestStores(t)
	_, err := newAsynx[domain.Arrow](adapter.Stores{Snapshots: stores.Snapshots})
	assert.Error(t, err)
}

func TestNewAsynx_MissingSnapshotStore_ReturnsError(t *testing.T) {
	stores := newTestStores(t)
	_, err := newAsynx[domain.Arrow](adapter.Stores{Events: stores.Events})
	assert.Error(t, err)
}

func TestNewAsynx_CorruptionHook_FallsBackToColdReplay(t *testing.T) {
	stores := newTestStores(t)
	ax, err := newAsynx[domain.Arrow](stores)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	ns := domain.Namespace("github.com/user/corrupt@v1")
	require.NoError(t, stores.Snapshots.Put(context.Background(), ns.String(), 1, []byte("not-json")))

	_, err = ax.Get(context.Background(), ns.String())
	require.Error(t, err)
	assert.ErrorIs(t, err, asynxModels.ErrNotFound)
}

func TestNewAsynx_PanicHandler_RecoversSubscriberPanic(t *testing.T) {
	ax, err := newAsynx[domain.Arrow](newTestStores(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	ns := domain.Namespace("github.com/user/panics@v1")
	_, err = ax.Subscribe(
		"arrow.added."+ns.String(),
		func(_ context.Context, _ asynxModels.Event[domain.Arrow]) {
			panic("boom")
		},
	)
	require.NoError(t, err)

	_, err = ax.Send(context.Background(), testAddArrowCmd{ns: ns})
	require.NoError(t, err)

	ax.WaitPublish()
}

func TestContainer_Shutdown_NilRepositories_ReturnsNil(t *testing.T) {
	c := &Container{}

	require.NoError(t, c.Shutdown(context.Background()))
}

func TestContainer_Shutdown_DelegatesToRepositories(t *testing.T) {
	home := t.TempDir()

	engines, err := engine.New(context.Background(), engine.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = engines.Shutdown(context.Background()) })

	adapters, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := New(engines, adapters, WithHomeDir(home))
	require.NoError(t, err)

	require.NoError(t, c.Shutdown(context.Background()))
	assert.Error(t, c.Shutdown(context.Background()),
		"the aggregates must report they are already drained on a second call")
}
