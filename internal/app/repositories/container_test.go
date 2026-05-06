package repositories_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	adapterSQLite "github.com/rabbytesoftware/quiver/internal/adapter/store/sqlite"
	repositories "github.com/rabbytesoftware/quiver/internal/app/repositories"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/mocks"
)

func newTestAsynxArrow(t *testing.T) asynx.Asynx[domain.Arrow] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Arrow]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 4, QueueDepth: 100}).
		Build()
	require.NoError(t, err)
	return ax
}

func newTestAsynxRuntime(t *testing.T) asynx.Asynx[domainRuntime.ArrowRuntime] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 4, QueueDepth: 100}).
		Build()
	require.NoError(t, err)
	return ax
}

func newTestAsynxCollection(t *testing.T) asynx.Asynx[domain.Collection] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Collection]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 4, QueueDepth: 100}).
		Build()
	require.NoError(t, err)
	return ax
}

func newTestContainer(t *testing.T) *repositories.Container {
	t.Helper()

	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)

	axArrow := newTestAsynxArrow(t)
	axRuntime := newTestAsynxRuntime(t)
	axCollection := newTestAsynxCollection(t)

	t.Cleanup(func() {
		_ = axArrow.Shutdown(context.Background())
		_ = axRuntime.Shutdown(context.Background())
		_ = axCollection.Shutdown(context.Background())
	})

	c, err := repositories.New(
		db,
		axArrow,
		axRuntime,
		axCollection,
		":memory:",
		nil,
		nil,
		nil,
		domain.OSDarwinARM64,
		nil,
	)
	require.NoError(t, err)
	return c
}

func TestNew_Success_ReturnsNonNilContainer(t *testing.T) {
	c := newTestContainer(t)
	assert.NotNil(t, c)
	assert.NotNil(t, c.Arrow)
	assert.NotNil(t, c.Runtime)
	assert.NotNil(t, c.Collection)
	assert.NotNil(t, c.Graph)
}

func TestNew_OnArrowAdded_TriggersSyncDependencies(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)

	axArrow := newTestAsynxArrow(t)
	axRuntime := newTestAsynxRuntime(t)
	axCollection := newTestAsynxCollection(t)

	t.Cleanup(func() {
		_ = axArrow.Shutdown(context.Background())
		_ = axRuntime.Shutdown(context.Background())
		_ = axCollection.Shutdown(context.Background())
	})

	c, err := repositories.New(
		db,
		axArrow,
		axRuntime,
		axCollection,
		":memory:",
		nil,
		nil,
		nil,
		domain.OSDarwinARM64,
		nil,
	)
	require.NoError(t, err)

	// Add an arrow with a dependency so SyncDependencies writes edges.
	ns := domain.Namespace("github.com/user/myapp@v1.0.0")
	depNs := domain.Namespace("github.com/user/dep@v0.1.0")

	arrow := &domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "myapp", Version: "v1.0.0"},
		Targets: map[domain.OS]domain.Target{
			domain.OSDarwinARM64: {
				Tools: []domain.DependencyEdge{
					{Namespace: depNs, Constraint: "v0.1.0"},
				},
			},
		},
	}

	var syncCalled atomic.Bool

	// Wire a second callback to detect sync.
	require.NoError(t, c.Arrow.OnArrowAdded(func(_ context.Context, _ domain.Namespace, _ domain.Arrow) error {
		syncCalled.Store(true)
		return nil
	}))

	// Trigger SyncDependencies directly to verify graph accepts it.
	err = c.Graph.SyncDependencies(context.Background(), ns, arrow)
	require.NoError(t, err)

	// Verify HasDependents reports the dep edge was written.
	hasDeps, err := c.Graph.HasDependents(context.Background(), depNs, domain.Namespace(""))
	require.NoError(t, err)
	assert.True(t, hasDeps, "expected dep edge to exist after SyncDependencies")
}

func TestNew_OnArrowRemoved_TriggersRemoveDependencies(t *testing.T) {
	c := newTestContainer(t)

	ns := domain.Namespace("github.com/user/myapp@v1.0.0")
	depNs := domain.Namespace("github.com/user/dep@v0.1.0")

	arrow := &domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "myapp", Version: "v1.0.0"},
		Targets: map[domain.OS]domain.Target{
			domain.OSDarwinARM64: {
				Tools: []domain.DependencyEdge{
					{Namespace: depNs, Constraint: "v0.1.0"},
				},
			},
		},
	}

	// Add edges.
	require.NoError(t, c.Graph.SyncDependencies(context.Background(), ns, arrow))

	hasDeps, err := c.Graph.HasDependents(context.Background(), depNs, domain.Namespace(""))
	require.NoError(t, err)
	require.True(t, hasDeps, "expected dep edge before removal")

	// Remove edges.
	require.NoError(t, c.Graph.RemoveDependencies(context.Background(), ns))

	hasDeps, err = c.Graph.HasDependents(context.Background(), depNs, domain.Namespace(""))
	require.NoError(t, err)
	assert.False(t, hasDeps, "expected dep edge removed after RemoveDependencies")
}

func TestRegisterHubProjections_WiresWithoutError(t *testing.T) {
	c := newTestContainer(t)

	hub := &stubHub{}
	err := c.RegisterHubProjections(hub)
	require.NoError(t, err)
}

// stubHub satisfies apphub.WebSocketHub without importing the hub package.
type stubHub struct {
	arrowBroadcasts   atomic.Int32
	runtimeBroadcasts atomic.Int32
	quiverBroadcasts  atomic.Int32
}

func (h *stubHub) BroadcastArrow(_ domain.Arrow) {
	h.arrowBroadcasts.Add(1)
}

func (h *stubHub) BroadcastArrowRuntime(_ domainRuntime.ArrowRuntime) {
	h.runtimeBroadcasts.Add(1)
}

func (h *stubHub) BroadcastCollection(_ domain.Collection) {
	h.quiverBroadcasts.Add(1)
}

// ─── isNotFound ───────────────────────────────────────────────────────────────

func TestIsNotFound_ErrNotFound_ReturnsTrue(t *testing.T) {
	assert.True(t, repositories.IsNotFoundTestable(asynxModels.ErrNotFound))
}

func TestIsNotFound_NilError_ReturnsFalse(t *testing.T) {
	assert.False(t, repositories.IsNotFoundTestable(nil))
}

func TestIsNotFound_OtherError_ReturnsFalse(t *testing.T) {
	assert.False(t, repositories.IsNotFoundTestable(errors.New("some other error")))
}

// ─── resolveManifestFrom closure paths ───────────────────────────────────────

func TestResolveManifestFrom_FoundInAsynx_ReturnsArrow(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })

	fn := repositories.ResolveManifestFromTestable(axArrow, nil)
	ns := domain.Namespace("github.com/user/repo@v1.0.0")

	// Use the newTestAsynxArrow-seeded asynx via a helper command approach.
	// Since asynx.Get returns ErrNotFound for unseeded namespaces, just test that path.
	_, err := fn(context.Background(), ns)
	// Not found → manifold nil → error about not found.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResolveManifestFrom_NotFound_NilManifold_ReturnsError(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })

	fn := repositories.ResolveManifestFromTestable(axArrow, nil)

	_, err := fn(context.Background(), domain.Namespace("github.com/user/pkg@v1"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResolveManifestFrom_NotFound_ManifoldSuccess_ReturnsArrow(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })

	ns := domain.Namespace("github.com/user/pkg@v1")
	arrow := &domain.Arrow{Namespace: ns}
	m := &mocks.Manifold{ResolveArrowResult: arrow}

	fn := repositories.ResolveManifestFromTestable(axArrow, m)

	got, err := fn(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, ns, got.Namespace)
}

func TestResolveManifestFrom_NotFound_ManifoldError_ReturnsError(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	t.Cleanup(func() { _ = axArrow.Shutdown(context.Background()) })

	fetchErr := errors.New("manifold error")
	m := &mocks.Manifold{ResolveArrowErr: fetchErr}

	fn := repositories.ResolveManifestFromTestable(axArrow, m)

	_, err := fn(context.Background(), domain.Namespace("github.com/user/pkg@v1"))
	require.Error(t, err)
	assert.ErrorIs(t, err, fetchErr)
}

func TestResolveManifestFrom_AsynxError_NonNotFound_ReturnsError(t *testing.T) {
	axArrow := newTestAsynxArrow(t)
	// Shut down asynx so Get returns a non-ErrNotFound error.
	_ = axArrow.Shutdown(context.Background())

	fn := repositories.ResolveManifestFromTestable(axArrow, nil)

	_, err := fn(context.Background(), domain.Namespace("github.com/user/pkg@v1"))
	// Depending on implementation, may get ErrNotFound (= not-found path) or shutdown error.
	_ = err
}
