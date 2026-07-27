package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormdb "gorm.io/gorm"

	"github.com/rabbytesoftware/quiver.core/internal/adapter"
	"github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/core/paths"
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

func TestContainer_Shutdown_ClosesArrowsReadModel(t *testing.T) {
	home := t.TempDir()

	engines, err := engine.New(context.Background(), engine.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = engines.Shutdown(context.Background()) })

	adapters, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := New(engines, adapters, WithHomeDir(home))
	require.NoError(t, err)

	sqlDB, err := c.arrowsDB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.PingContext(context.Background()))

	require.NoError(t, c.Shutdown(context.Background()))

	assert.Error(t, sqlDB.PingContext(context.Background()),
		"arrows.db must be closed once every aggregate has drained")
}

func TestDiscardRepos_ReleasesArrowsHandleAndRepositories(t *testing.T) {
	home := t.TempDir()

	engines, err := engine.New(context.Background(), engine.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = engines.Shutdown(context.Background()) })

	adapters, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := New(engines, adapters, WithHomeDir(home))
	require.NoError(t, err)

	sqlDB, err := c.arrowsDB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.PingContext(context.Background()))

	discardRepos(c.repos, c.arrowsDB)

	assert.Error(t, sqlDB.PingContext(context.Background()),
		"a failed construction must release the arrows handle")
	assert.Error(t,
		c.repos.Collection.Follow(context.Background(), "github.com/org/repo", &domain.Collection{}, nil),
		"a failed construction must release the repositories it already built")

	// Releasing what is already released reports rather than panics.
	discardRepos(c.repos, c.arrowsDB)
}

func TestDiscardDB_UnusableHandle_DoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() { discardDB(&gormdb.DB{Config: &gormdb.Config{}}) })
}

func TestNew_CollectionStoreUnopenable_ReturnsError(t *testing.T) {
	home := t.TempDir()

	engines, err := engine.New(context.Background(), engine.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = engines.Shutdown(context.Background()) })

	adapters, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	storePath, err := paths.StoreAt(home)
	require.NoError(t, err)
	// A directory where the collections database belongs fails the sqlite open
	// while leaving arrows.db openable, so New fails after taking that handle.
	require.NoError(t, os.Mkdir(filepath.Join(storePath, "collections.db"), 0o755))

	_, err = New(engines, adapters, WithHomeDir(home))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "app container: repositories")
}

func TestContainer_Shutdown_ArrowsDBUnusable_ReturnsCloseError(t *testing.T) {
	c := &Container{arrowsDB: &gormdb.DB{Config: &gormdb.Config{}}}

	err := c.Shutdown(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "close arrows db")
}

// release hands the container back the way the daemon does. Closing the
// adapters is not enough: the container opens arrows.db and collections.db
// itself, and only Shutdown owns those. An open SQLite file is one t.TempDir
// cannot remove on Windows, so a container a test builds is a container it has
// to shut down. Shutdown is safe to call twice, so tests may still call it
// themselves.
func release(
	t *testing.T,
	c *Container,
) {
	t.Helper()
	t.Cleanup(func() { _ = c.Shutdown(context.Background()) })
}

// newContainer wires the real thing on a temp home: the container's job is to
// compose engines and adapters, and a stubbed engine would not exercise that.
func newContainer(t *testing.T) *Container {
	t.Helper()

	home := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	engines, err := engine.New(ctx, engine.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = engines.Shutdown(context.Background()) })

	adapters, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	c, err := New(engines, adapters, WithHomeDir(home))
	require.NoError(t, err)
	release(t, c)
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
	t.Cleanup(func() { _ = engines.Shutdown(context.Background()) })

	_, err = New(engines, &adapter.Container{}, WithHomeDir(home))
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
	t.Cleanup(func() { _ = engines.Shutdown(context.Background()) })

	adapters, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	notADir := filepath.Join(home, "file")
	require.NoError(t, os.WriteFile(notADir, []byte("x"), 0o600))

	_, err = New(engines, adapters, WithHomeDir(notADir))
	require.Error(t, err)
}

func TestContainer_StartAndShutdown(t *testing.T) {
	c := newContainer(t)

	ctx := context.Background()
	c.Start(ctx)

	require.NoError(t, c.Shutdown(ctx))
}
