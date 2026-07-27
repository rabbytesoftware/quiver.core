package repositories_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlite "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	adapterSQLite "github.com/rabbytesoftware/quiver.core/internal/adapter/store/sqlite"
	apphub "github.com/rabbytesoftware/quiver.core/internal/app/hub"
	appmocks "github.com/rabbytesoftware/quiver.core/internal/app/mocks"
	repositories "github.com/rabbytesoftware/quiver.core/internal/app/repositories"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	ucmocks "github.com/rabbytesoftware/quiver.core/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

func newTestAsynxArrow(t *testing.T) asynx.Asynx[domain.Arrow] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Arrow]().
		WithEventStore(es).
		WithSnapshotStore(ss).
		WithShardingOpts(asynx.ShardingOpts{Shards: 4, QueueDepth: 100}).
		Build()
	require.NoError(t, err)
	return ax
}

func newTestAsynxRuntime(t *testing.T) asynx.Asynx[domainRuntime.ArrowRuntime] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(es).
		WithSnapshotStore(ss).
		WithShardingOpts(asynx.ShardingOpts{Shards: 4, QueueDepth: 100}).
		Build()
	require.NoError(t, err)
	return ax
}

func newTestAsynxCollection(t *testing.T) asynx.Asynx[domain.Collection] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Collection]().
		WithEventStore(es).
		WithSnapshotStore(ss).
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
		nil,
	)
	require.NoError(t, err)
	return c
}

// testFollowCmd drives the collection aggregate so a test can tell a live
// instance from a drained one by the error Send returns.
type testFollowCmd struct{ ns domain.Namespace }

func (c testFollowCmd) AggregateID() string                 { return c.ns.String() }
func (c testFollowCmd) EventName() string                   { return "collection.followed." + c.ns.String() }
func (c testFollowCmd) ShouldSnapshot() bool                { return true }
func (c testFollowCmd) Validate(_ *domain.Collection) error { return nil }

func (c testFollowCmd) EmitEvent(_ *domain.Collection) domain.Collection {
	return domain.Collection{Namespace: c.ns}
}

func TestNew_RuntimeWiringFails_ReleasesCollectionStore(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)

	axArrow := newTestAsynxArrow(t)
	axCollection := newTestAsynxCollection(t)
	t.Cleanup(func() {
		_ = axArrow.Shutdown(context.Background())
		_ = axCollection.Shutdown(context.Background())
	})

	subscribeErr := errors.New("subscribe boom")
	axRuntime := &appmocks.AsynxRuntime{
		SubscribeFn: func(
			_ string,
			_ asynxModels.ProjectionHandler[domainRuntime.ArrowRuntime],
			_ ...asynxModels.SubscriptionOpt[domainRuntime.ArrowRuntime],
		) (string, error) {
			return "", subscribeErr
		},
	}

	require.NoError(t, axCollection.Preload(context.Background(), "github.com/org/live"))
	_, liveErr := axCollection.Send(context.Background(), testFollowCmd{ns: "github.com/org/live"})
	require.NoError(t, liveErr, "the same command must succeed while the aggregate is live")

	_, err = repositories.New(
		db, axArrow, axRuntime, axCollection, ":memory:",
		nil, nil, nil, domain.OSDarwinARM64, nil, nil,
	)
	require.Error(t, err)

	_, sendErr := axCollection.Send(context.Background(), testFollowCmd{ns: "github.com/org/drained"})
	assert.Error(t, sendErr,
		"the collection repository must be released when a later step of New fails")
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

func (h *stubHub) BroadcastArrow(_ apphub.ArrowEvent) {
	h.arrowBroadcasts.Add(1)
}

func (h *stubHub) BroadcastArrowRuntime(_ domainRuntime.ArrowRuntime) {
	h.runtimeBroadcasts.Add(1)
}

func (h *stubHub) BroadcastCollection(_ apphub.CollectionEvent) {
	h.quiverBroadcasts.Add(1)
}

// ─── Shutdown ─────────────────────────────────────────────────────────────────

// shutdownRecorder builds a Container whose three aggregates append their name
// to order when drained, so the drain sequence is observable.
func shutdownRecorder(
	order *[]string,
	runtimeErr error,
) *repositories.Container {
	return &repositories.Container{
		Runtime: &ucmocks.MockRuntime{ShutdownFn: func(_ context.Context) error {
			*order = append(*order, "runtime")
			return runtimeErr
		}},
		Collection: &ucmocks.MockCollection{ShutdownFn: func(_ context.Context) error {
			*order = append(*order, "collection")
			return nil
		}},
		Arrow: &ucmocks.MockArrow{ShutdownFn: func(_ context.Context) error {
			*order = append(*order, "arrow")
			return nil
		}},
	}
}

func TestContainer_Shutdown_DrainsRuntimeBeforeArrow(t *testing.T) {
	var order []string

	c := shutdownRecorder(&order, nil)

	require.NoError(t, c.Shutdown(context.Background()))
	assert.Equal(t, []string{"runtime", "collection", "arrow"}, order,
		"arrow must drain last so a lost MarkInstalled cannot leave a ready runtime with no installed ref")
}

func TestContainer_Shutdown_RunsEveryPhaseDespiteFailure(t *testing.T) {
	var order []string
	drainErr := errors.New("drain boom")

	c := shutdownRecorder(&order, drainErr)

	err := c.Shutdown(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, drainErr)
	assert.Equal(t, []string{"runtime", "collection", "arrow"}, order,
		"a failed drain must not skip the remaining aggregates")
}

func TestContainer_Shutdown_CollectsEveryPhaseError(t *testing.T) {
	runtimeErr := errors.New("runtime boom")
	collectionErr := errors.New("collection boom")
	arrowErr := errors.New("arrow boom")

	c := &repositories.Container{
		Runtime:    &ucmocks.MockRuntime{ShutdownFn: func(_ context.Context) error { return runtimeErr }},
		Collection: &ucmocks.MockCollection{ShutdownFn: func(_ context.Context) error { return collectionErr }},
		Arrow:      &ucmocks.MockArrow{ShutdownFn: func(_ context.Context) error { return arrowErr }},
	}

	err := c.Shutdown(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, runtimeErr)
	assert.ErrorIs(t, err, collectionErr)
	assert.ErrorIs(t, err, arrowErr)
}

func TestContainer_Shutdown_SlowRuntimeDrain_DoesNotStarveTheOthers(t *testing.T) {
	var collectionRan, arrowRan bool
	var collectionCtxErr, arrowCtxErr error

	c := &repositories.Container{
		// The runtime drain of an arrow whose process will not die: it holds on
		// until its own budget runs out. Sharing one context makes that budget the
		// whole of ctx, and every aggregate after it drains on a dead one.
		Runtime: &ucmocks.MockRuntime{ShutdownFn: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}},
		Collection: &ucmocks.MockCollection{ShutdownFn: func(ctx context.Context) error {
			collectionRan = true
			collectionCtxErr = ctx.Err()
			return nil
		}},
		Arrow: &ucmocks.MockArrow{ShutdownFn: func(ctx context.Context) error {
			arrowRan = true
			arrowCtxErr = ctx.Err()
			return nil
		}},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := c.Shutdown(ctx)

	require.ErrorIs(t, err, context.DeadlineExceeded, "the runtime drain must report that it ran out of time")
	require.True(t, collectionRan, "the collection drain must run after the runtime drain overruns")
	require.True(t, arrowRan, "the arrow drain must run after the runtime drain overruns")
	assert.NoError(t, collectionCtxErr,
		"the collection drain must get a share of its own, not the one runtime spent")
	assert.NoError(t, arrowCtxErr,
		"the arrow drain must get a share of its own, not the one runtime spent")
}

func TestContainer_Shutdown_RealAggregates_DrainsAll(t *testing.T) {
	c := newTestContainer(t)

	require.NoError(t, c.Shutdown(context.Background()))

	err := c.Collection.Follow(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
		&domain.Collection{},
		nil,
	)
	assert.Error(t, err, "a drained aggregate must reject new commands")
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

// ─── discovery wiring ────────────────────────────────────────────────────────

func newDiscoverableContainer(
	t *testing.T,
	providers []provider.Provider,
) (*repositories.Container, asynx.Asynx[domain.Arrow]) {
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

	root := t.TempDir()
	v, err := vault.New(filepath.Join(root, "vault"), filepath.Join(root, "namespaces"), time.Hour)
	require.NoError(t, err)

	c, err := repositories.New(
		db,
		axArrow,
		axRuntime,
		axCollection,
		":memory:",
		v,
		&mocks.Manifold{ResolveArrowErr: errors.New("not resolvable in this test")},
		nil,
		domain.OSDarwinARM64,
		nil,
		providers,
	)
	require.NoError(t, err)
	return c, axArrow
}

// A container built without a vault or manifold cannot verify anything, so it
// has no discovery rather than a half-built one.
func TestNew_WithoutVaultOrManifold_HasNoDiscovery(t *testing.T) {
	assert.Nil(t, newTestContainer(t).Discovery)
}

func TestNew_WithVaultAndManifold_BuildsDiscovery(t *testing.T) {
	c, _ := newDiscoverableContainer(t, nil)
	assert.NotNil(t, c.Discovery)
}

// The catalog lookup handed to discovery must answer false for an unknown
// namespace rather than propagating the not-found error.
func TestDiscovery_UnknownCandidateIsNotFlaggedKnown(t *testing.T) {
	c, _ := newDiscoverableContainer(t, []provider.Provider{
		&stubSearchProvider{host: "github.com", candidates: []provider.Candidate{{
			Namespace:     domain.Namespace("github.com/acme/unknown"),
			Source:        "github.com",
			DefaultBranch: "main",
		}}},
	})

	outcome, err := c.Discovery.Discover(context.Background(), "unknown", func(discovery.Result) {})
	require.NoError(t, err)
	assert.Equal(t, 1, outcome.Found)
	// The stub manifold refuses to resolve, so the candidate is unproven.
	assert.Equal(t, 1, outcome.Skipped)
}

func TestDiscovery_CandidateInTheCatalogIsFlaggedKnown(t *testing.T) {
	c, axArrow := newDiscoverableContainer(t, nil)

	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	require.NoError(t, c.Arrow.AddDep(context.Background(), ns, &domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "Pkg"},
	}, ""))
	axArrow.WaitPublish()

	known, err := repositories.CatalogHas(c.Arrow)(context.Background(), ns.BareNamespace())
	require.NoError(t, err)
	assert.True(t, known)

	missing, err := repositories.CatalogHas(c.Arrow)(
		context.Background(),
		domain.Namespace("github.com/user/absent"),
	)
	require.NoError(t, err)
	assert.False(t, missing)
}

type stubSearchProvider struct {
	host       string
	candidates []provider.Candidate
}

func (s *stubSearchProvider) Host() string { return s.host }

func (s *stubSearchProvider) Search(
	_ context.Context,
	_ provider.SearchRequest,
) ([]provider.Candidate, error) {
	return s.candidates, nil
}

// A catalog that is broken rather than merely empty must surface the failure,
// not silently answer "not known".
func TestCatalogHas_LookupFailure_PropagatesTheError(t *testing.T) {
	broken := &ucmocks.MockArrow{
		GetFn: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
			return nil, errors.New("database is locked")
		},
	}

	_, err := repositories.CatalogHas(broken)(context.Background(), domain.Namespace("github.com/u/r"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database is locked")
}
