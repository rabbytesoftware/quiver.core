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
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	repositories "github.com/rabbytesoftware/quiver.core/internal/app/repositories"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	ucmocks "github.com/rabbytesoftware/quiver.core/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	authdomain "github.com/rabbytesoftware/quiver.core/internal/domain/auth"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
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

func newTestAsynxPairingCode(t *testing.T) asynx.Asynx[authdomain.PairingCode] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[authdomain.PairingCode]().
		WithEventStore(es).
		WithSnapshotStore(ss).
		WithShardingOpts(asynx.ShardingOpts{Shards: 2, QueueDepth: 100}).
		Build()
	require.NoError(t, err)
	return ax
}

func newTestAsynxDevice(t *testing.T) asynx.Asynx[authdomain.Device] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := sqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[authdomain.Device]().
		WithEventStore(es).
		WithSnapshotStore(ss).
		WithShardingOpts(asynx.ShardingOpts{Shards: 2, QueueDepth: 100}).
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
	axPairingCode := newTestAsynxPairingCode(t)
	axDevice := newTestAsynxDevice(t)

	t.Cleanup(func() {
		_ = axArrow.Shutdown(context.Background())
		_ = axRuntime.Shutdown(context.Background())
		_ = axCollection.Shutdown(context.Background())
		_ = axPairingCode.Shutdown(context.Background())
		_ = axDevice.Shutdown(context.Background())
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
		axPairingCode,
		axDevice,
		db,
	)
	require.NoError(t, err)
	return c
}

// newTestContainerWithVaultAndManifold builds a container with a real vault
// and an injected manifold, so tests can assert on the manifold call count a
// resolution path actually produces.
func newTestContainerWithVaultAndManifold(
	t *testing.T,
	v vault.Vault,
	m manifold.Manifold,
) *repositories.Container {
	t.Helper()

	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)

	axArrow := newTestAsynxArrow(t)
	axRuntime := newTestAsynxRuntime(t)
	axCollection := newTestAsynxCollection(t)
	axPairingCode := newTestAsynxPairingCode(t)
	axDevice := newTestAsynxDevice(t)

	t.Cleanup(func() {
		_ = axArrow.Shutdown(context.Background())
		_ = axRuntime.Shutdown(context.Background())
		_ = axCollection.Shutdown(context.Background())
		_ = axPairingCode.Shutdown(context.Background())
		_ = axDevice.Shutdown(context.Background())
	})

	c, err := repositories.New(
		db,
		axArrow,
		axRuntime,
		axCollection,
		":memory:",
		v,
		m,
		nil,
		domain.OSDarwinARM64,
		nil,
		nil,
		axPairingCode,
		axDevice,
		db,
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
		nil, nil, nil,
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
	assert.NotNil(t, c.Cascade)
}

func TestNew_OnArrowAdded_TriggersSyncDependencies(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)

	axArrow := newTestAsynxArrow(t)
	axRuntime := newTestAsynxRuntime(t)
	axCollection := newTestAsynxCollection(t)
	axPairingCode := newTestAsynxPairingCode(t)
	axDevice := newTestAsynxDevice(t)

	t.Cleanup(func() {
		_ = axArrow.Shutdown(context.Background())
		_ = axRuntime.Shutdown(context.Background())
		_ = axCollection.Shutdown(context.Background())
		_ = axPairingCode.Shutdown(context.Background())
		_ = axDevice.Shutdown(context.Background())
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
		axPairingCode,
		axDevice,
		db,
	)
	require.NoError(t, err)

	// Add an arrow with a dependency so SyncDependencies writes edges.
	ns := domain.Namespace("github.com/user/myapp@v1.0.0")
	depNs := domain.Namespace("github.com/user/dep@v0.1.0")

	arrow := &domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "myapp"},
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
		ArrowMeta: domain.ArrowMeta{Name: "myapp"},
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
		Cascade: &ucmocks.MockCascade{ShutdownFn: func(_ context.Context) error {
			*order = append(*order, "cascade")
			return nil
		}},
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
		PairingCode: &ucmocks.MockPairingCode{ShutdownFn: func(_ context.Context) error {
			*order = append(*order, "pairingcode")
			return nil
		}},
		Device: &ucmocks.MockDevice{ShutdownFn: func(_ context.Context) error {
			*order = append(*order, "device")
			return nil
		}},
	}
}

func TestContainer_Shutdown_DrainsRuntimeBeforeArrow(t *testing.T) {
	var order []string

	c := shutdownRecorder(&order, nil)

	require.NoError(t, c.Shutdown(context.Background()))
	assert.Equal(t, []string{"cascade", "runtime", "collection", "arrow", "pairingcode", "device"}, order,
		"arrow must drain last among the arrow-related aggregates so a lost MarkInstalled cannot leave a ready runtime with no installed ref")
}

func TestContainer_Shutdown_RunsEveryPhaseDespiteFailure(t *testing.T) {
	var order []string
	drainErr := errors.New("drain boom")

	c := shutdownRecorder(&order, drainErr)

	err := c.Shutdown(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, drainErr)
	assert.Equal(t, []string{"cascade", "runtime", "collection", "arrow", "pairingcode", "device"}, order,
		"a failed drain must not skip the remaining aggregates")
}

func TestContainer_Shutdown_CollectsEveryPhaseError(t *testing.T) {
	runtimeErr := errors.New("runtime boom")
	collectionErr := errors.New("collection boom")
	arrowErr := errors.New("arrow boom")

	c := &repositories.Container{
		Cascade:     &ucmocks.MockCascade{},
		Runtime:     &ucmocks.MockRuntime{ShutdownFn: func(_ context.Context) error { return runtimeErr }},
		Collection:  &ucmocks.MockCollection{ShutdownFn: func(_ context.Context) error { return collectionErr }},
		Arrow:       &ucmocks.MockArrow{ShutdownFn: func(_ context.Context) error { return arrowErr }},
		PairingCode: &ucmocks.MockPairingCode{},
		Device:      &ucmocks.MockDevice{},
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
		Cascade: &ucmocks.MockCascade{},
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
		PairingCode: &ucmocks.MockPairingCode{},
		Device:      &ucmocks.MockDevice{},
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

// ─── RecoverForgetCascade ───────────────────────────────────────────────────────

func TestRecoverForgetCascade_DrainsTheCascade(t *testing.T) {
	var drained bool
	c := &repositories.Container{
		Cascade: &ucmocks.MockCascade{DrainFn: func(_ context.Context) error {
			drained = true
			return nil
		}},
	}

	c.RecoverForgetCascade(context.Background())
	assert.True(t, drained, "boot recovery must drain the cascade queue")
}

func TestRecoverForgetCascade_DrainError_DoesNotPanic(t *testing.T) {
	c := &repositories.Container{
		Cascade: &ucmocks.MockCascade{DrainFn: func(_ context.Context) error {
			return errors.New("drain boom")
		}},
	}

	assert.NotPanics(t, func() { c.RecoverForgetCascade(context.Background()) })
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
	axPairingCode := newTestAsynxPairingCode(t)
	axDevice := newTestAsynxDevice(t)

	t.Cleanup(func() {
		_ = axArrow.Shutdown(context.Background())
		_ = axRuntime.Shutdown(context.Background())
		_ = axCollection.Shutdown(context.Background())
		_ = axPairingCode.Shutdown(context.Background())
		_ = axDevice.Shutdown(context.Background())
	})

	root := t.TempDir()
	v, err := vault.New(filepath.Join(root, "vault"), filepath.Join(root, "namespaces"), time.Hour)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })

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
		axPairingCode,
		axDevice,
		db,
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

func (s *stubSearchProvider) CanSearch() bool { return true }

func (s *stubSearchProvider) Search(
	_ context.Context,
	_ provider.SearchRequest,
) ([]provider.Candidate, error) {
	return s.candidates, nil
}

// The host questions below complete the provider contract. Discovery asks a
// provider to search and nothing else.
func (s *stubSearchProvider) LatestRelease(
	_ context.Context,
	_ domain.Namespace,
) (string, error) {
	return "", errNotSearch
}

func (s *stubSearchProvider) RawFileURL(
	_ domain.Namespace,
	_ string,
	_ string,
) (string, error) {
	return "", errNotSearch
}

func (s *stubSearchProvider) DefaultBranches() []string { return nil }

var errNotSearch = errors.New("stub provider: discovery never asks this")

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

// ─── Dependency edges are in place before an arrow can be acted on ───────────

// depArrow is an arrow whose only interesting property is that it depends on
// depNs for the platform the test container runs as.
func depArrow(
	ns domain.Namespace,
	depNs domain.Namespace,
) *domain.Arrow {
	return &domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "parent"},
		Targets: map[domain.OS]domain.Target{
			domain.OSDarwinARM64: {
				Tools: []domain.DependencyEdge{
					{Namespace: depNs, Constraint: depNs.Ref(), Type: domain.ToolDep},
				},
			},
		},
	}
}

// The caller's next move after adding an arrow is to install it, and installing
// walks the dependency edges the add produced. An add that returns before those
// edges exist lets a dependency be removed while something still needs it —
// the remove guard reads the edge table and finds nothing.
func TestAdd_DependencyEdgesExistWhenAddReturns(t *testing.T) {
	c := newTestContainer(t)

	ns := domain.Namespace("github.com/user/parent@v1.0.0")
	depNs := domain.Namespace("github.com/user/dep@v0.1.0")

	require.NoError(t, c.Arrow.AddDep(context.Background(), ns, depArrow(ns, depNs), depNs.Ref()))

	hasDeps, err := c.Graph.HasDependents(context.Background(), depNs, domain.Namespace(""))
	require.NoError(t, err)
	assert.True(t, hasDeps,
		"the dependency edge must exist by the time AddDep returns")
}

// The catalog row is the other half of the same invariant: an arrow that can be
// read must already have its edges.
func TestAdd_ArrowIsReadableWhenAddReturns(t *testing.T) {
	c := newTestContainer(t)

	ns := domain.Namespace("github.com/user/parent@v1.0.0")
	depNs := domain.Namespace("github.com/user/dep@v0.1.0")

	require.NoError(t, c.Arrow.AddDep(context.Background(), ns, depArrow(ns, depNs), depNs.Ref()))

	got, err := c.Arrow.Get(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "parent", got.Name)
}

// A changed manifest changes the edges, and the caller reads them back.
func TestUpdateManifest_DependencyEdgesExistWhenUpdateReturns(t *testing.T) {
	c := newTestContainer(t)

	ns := domain.Namespace("github.com/user/parent@v1.0.0")
	firstDep := domain.Namespace("github.com/user/dep-a@v0.1.0")
	secondDep := domain.Namespace("github.com/user/dep-b@v0.1.0")

	require.NoError(t, c.Arrow.AddDep(context.Background(), ns, depArrow(ns, firstDep), firstDep.Ref()))
	require.NoError(t, c.Arrow.UpdateManifest(context.Background(), ns, depArrow(ns, secondDep)))

	hasFirst, err := c.Graph.HasDependents(context.Background(), firstDep, domain.Namespace(""))
	require.NoError(t, err)
	assert.False(t, hasFirst, "the dropped dependency must no longer have a dependent")

	hasSecond, err := c.Graph.HasDependents(context.Background(), secondDep, domain.Namespace(""))
	require.NoError(t, err)
	assert.True(t, hasSecond, "the new dependency must have one by the time the update returns")
}

// Removing an arrow tears its edges down, so what depended on it is free again.
func TestRemove_DependencyEdgesGoneWhenRemoveReturns(t *testing.T) {
	c := newTestContainer(t)

	ns := domain.Namespace("github.com/user/parent@v1.0.0")
	depNs := domain.Namespace("github.com/user/dep@v0.1.0")

	require.NoError(t, c.Arrow.AddDep(context.Background(), ns, depArrow(ns, depNs), depNs.Ref()))
	require.NoError(t, c.Arrow.Remove(context.Background(), ns))

	hasDeps, err := c.Graph.HasDependents(context.Background(), depNs, domain.Namespace(""))
	require.NoError(t, err)
	assert.False(t, hasDeps, "the edge must be gone by the time Remove returns")

	_, err = c.Arrow.Get(context.Background(), ns)
	require.Error(t, err)
}

// The edge rows carry the type the manifest declared, not a default. Two
// writers used to race for these rows with different contents, so which type
// was stored depended on which goroutine finished last.
func TestSyncDependencies_RecordsDeclaredDepType(t *testing.T) {
	c := newTestContainer(t)

	ns := domain.Namespace("github.com/user/parent@v1.0.0")
	svcNs := domain.Namespace("github.com/user/svc@v0.1.0")

	arrow := &domain.Arrow{
		Namespace: ns,
		Targets: map[domain.OS]domain.Target{
			domain.OSDarwinARM64: {
				Services: []domain.DependencyEdge{
					{Namespace: svcNs, Constraint: svcNs.Ref(), Type: domain.ServiceDep},
				},
			},
		},
	}
	require.NoError(t, c.Arrow.AddDep(context.Background(), ns, arrow, svcNs.Ref()))

	dependents, err := c.Graph.GetDependents(context.Background(), svcNs)
	require.NoError(t, err)
	assert.Equal(t, []domain.Namespace{ns}, dependents)
}

// Graph.Resolve must resolve the root manifest the same vault-cached way
// GetManifest/GetReadme/GetDetail do. Before this was wired through
// cat.ResolveManifest, an uncatalogued namespace's /dependencies call paid its
// own live manifold fetch even seconds after /manifest had just cached the
// exact same manifest.
func TestGetDependencies_UncataloguedNamespace_ReusesVaultCache(t *testing.T) {
	dir := t.TempDir()
	v, err := vault.New(filepath.Join(dir, "vault"), filepath.Join(dir, "ns"), time.Hour)
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })

	ns := domain.Namespace("github.com/char2cs/crowbar@develop")
	arrow := &domain.Arrow{
		Namespace: ns,
		ArrowMeta: domain.ArrowMeta{Name: "crowbar"},
		Targets:   map[domain.OS]domain.Target{domain.OSDarwinARM64: {}},
	}

	var resolveCalls int32
	m := &mocks.Manifold{
		// ParseArrowResult backs the cache-hit path: the second call reads the
		// vault's cached bytes back through ParseArrow rather than ResolveArrow.
		ParseArrowResult: arrow,
		ResolveArrowFunc: func(_ context.Context, _ domain.Namespace) (*domain.Arrow, []byte, string, error) {
			atomic.AddInt32(&resolveCalls, 1)
			return arrow, []byte("raw"), "ARROW.md", nil
		},
	}

	c := newTestContainerWithVaultAndManifold(t, v, m)

	// Mirrors GET /manifest: warms the vault cache with a live fetch.
	_, err = c.Arrow.ResolveManifest(context.Background(), ns)
	require.NoError(t, err)
	require.EqualValues(t, 1, atomic.LoadInt32(&resolveCalls))

	// Mirrors GET /dependencies for the same uncatalogued namespace: must
	// reuse the manifest /manifest just cached, not fetch it again.
	_, err = c.Graph.Resolve(context.Background(), ns)
	require.NoError(t, err)
	assert.EqualValues(t, 1, atomic.LoadInt32(&resolveCalls),
		"graph.Resolve must reuse the vault cache instead of a fresh manifold fetch")
}

// ─── wireCallbacks ───────────────────────────────────────────────────────────

func TestWireCallbacks_PropagatesRegistrationErrors(t *testing.T) {
	boom := errors.New("register failed")

	testCases := []struct {
		name  string
		arrow *ucmocks.MockArrow
		want  string
	}{
		{
			name: "added",
			arrow: &ucmocks.MockArrow{
				OnArrowAddedFn: func(_ func(context.Context, domain.Namespace, domain.Arrow) error) error {
					return boom
				},
			},
			want: "repositories: wire OnArrowAdded",
		},
		{
			name: "updated",
			arrow: &ucmocks.MockArrow{
				OnArrowUpdatedFn: func(_ func(context.Context, domain.Namespace, *domain.Arrow) error) error {
					return boom
				},
			},
			want: "repositories: wire OnArrowUpdated",
		},
		{
			name: "upgraded",
			arrow: &ucmocks.MockArrow{
				OnArrowUpgradedFn: func(_ func(context.Context, domain.Arrow) error) error {
					return boom
				},
			},
			want: "repositories: wire OnArrowUpgraded",
		},
		{
			name: "removed",
			arrow: &ucmocks.MockArrow{
				OnArrowRemovedFn: func(_ func(context.Context, domain.Namespace) error) error {
					return boom
				},
			},
			want: "repositories: wire OnArrowRemoved",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := &repositories.Container{Arrow: tc.arrow}

			err := c.WireCallbacks()

			require.Error(t, err)
			assert.ErrorIs(t, err, boom)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// The wiring is what the invariant is made of, so the registered reactions have
// to reach the graph — and, for removal, the cascade queue — and in that order.
func TestWireCallbacks_RegisteredReactionsDriveGraphAndCascade(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")

	var (
		added    func(context.Context, domain.Namespace, domain.Arrow) error
		updated  func(context.Context, domain.Namespace, *domain.Arrow) error
		upgraded func(context.Context, domain.Arrow) error
		removed  func(context.Context, domain.Namespace) error
	)

	arrow := &ucmocks.MockArrow{
		OnArrowAddedFn: func(fn func(context.Context, domain.Namespace, domain.Arrow) error) error {
			added = fn
			return nil
		},
		OnArrowUpdatedFn: func(fn func(context.Context, domain.Namespace, *domain.Arrow) error) error {
			updated = fn
			return nil
		},
		OnArrowUpgradedFn: func(fn func(context.Context, domain.Arrow) error) error {
			upgraded = fn
			return nil
		},
		OnArrowRemovedFn: func(fn func(context.Context, domain.Namespace) error) error {
			removed = fn
			return nil
		},
	}

	var order []string
	syncErr := errors.New("sync failed")
	graphMock := &ucmocks.MockGraph{
		SyncDependenciesFn: func(_ context.Context, _ domain.Namespace, _ *domain.Arrow) error {
			order = append(order, "sync")
			return nil
		},
		RemoveDependenciesFn: func(_ context.Context, _ domain.Namespace) error {
			order = append(order, "remove edges")
			return nil
		},
	}
	cascadeMock := &ucmocks.MockCascade{
		EnqueueFn: func(_ context.Context, _ domain.Namespace) error {
			order = append(order, "enqueue cascade")
			return nil
		},
	}

	c := &repositories.Container{Arrow: arrow, Graph: graphMock, Cascade: cascadeMock}
	require.NoError(t, c.WireCallbacks())

	require.NoError(t, added(context.Background(), ns, domain.Arrow{Namespace: ns}))
	require.NoError(t, updated(context.Background(), ns, &domain.Arrow{Namespace: ns}))
	require.NoError(t, upgraded(context.Background(), domain.Arrow{Namespace: ns}))
	require.NoError(t, removed(context.Background(), ns))

	assert.Equal(t, []string{"sync", "sync", "sync", "remove edges", "enqueue cascade"}, order)

	// A graph that cannot drop the edges must stop the cascade: enqueueing the
	// runtime for forgetting would leave the edges pointing at an arrow nobody
	// owns.
	order = nil
	graphMock.RemoveDependenciesFn = func(_ context.Context, _ domain.Namespace) error {
		return syncErr
	}
	require.ErrorIs(t, removed(context.Background(), ns), syncErr)
	assert.Empty(t, order)
}

// ─── RegisterHubProjections ──────────────────────────────────────────────────

func TestRegisterHubProjections_PropagatesRegistrationErrors(t *testing.T) {
	boom := errors.New("register failed")

	runtimeHook := func(name string) *ucmocks.MockRuntime {
		fail := func(_ func(context.Context, domainRuntime.ArrowRuntime)) error { return boom }
		m := &ucmocks.MockRuntime{}
		switch name {
		case "begun":
			m.OnRuntimeBegunFn = fail
		case "ended":
			m.OnRuntimeEndedFn = fail
		case "recovered":
			m.OnRuntimeRecoveredFn = fail
		case "detached":
			m.OnRuntimeDetachedFn = fail
		case "pid":
			m.OnRuntimePIDRecordedFn = fail
		case "outdated":
			m.OnRuntimeOutdatedFn = fail
		case "outdated cleared":
			m.OnRuntimeOutdatedClearedFn = fail
		case "step advanced":
			m.OnRuntimeStepAdvancedFn = fail
		}
		return m
	}

	testCases := []struct {
		name       string
		collection *ucmocks.MockCollection
		want       string
	}{
		{name: "begun", want: "hub OnRuntimeBegun"},
		{name: "ended", want: "hub OnRuntimeEnded"},
		{name: "recovered", want: "hub OnRuntimeRecovered"},
		{name: "detached", want: "hub OnRuntimeDetached"},
		{name: "pid", want: "hub OnRuntimePIDRecorded"},
		{name: "outdated", want: "hub OnRuntimeOutdated"},
		{name: "outdated cleared", want: "hub OnRuntimeOutdatedCleared"},
		{name: "step advanced", want: "hub OnRuntimeStepAdvanced"},
		{
			name: "collection followed",
			collection: &ucmocks.MockCollection{
				OnCollectionFollowedFn: func(_ func(context.Context, domain.Collection)) error {
					return boom
				},
			},
			want: "hub OnCollectionFollowed",
		},
		{
			name: "collection unfollowed",
			collection: &ucmocks.MockCollection{
				OnCollectionUnfollowedFn: func(_ func(context.Context, domain.Namespace)) error {
					return boom
				},
			},
			want: "hub OnCollectionUnfollowed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			coll := tc.collection
			if coll == nil {
				coll = &ucmocks.MockCollection{}
			}
			c := &repositories.Container{
				Runtime:    runtimeHook(tc.name),
				Collection: coll,
			}

			err := c.RegisterHubProjections(&stubHub{})

			require.Error(t, err)
			assert.ErrorIs(t, err, boom)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// Every registered hook must reach the hub, or a client never learns the state
// changed.
func TestRegisterHubProjections_RegisteredHooksBroadcast(t *testing.T) {
	var (
		runtimeHooks   []func(context.Context, domainRuntime.ArrowRuntime)
		followedHook   func(context.Context, domain.Collection)
		unfollowedHook func(context.Context, domain.Namespace)
	)

	captureRuntime := func(fn func(context.Context, domainRuntime.ArrowRuntime)) error {
		runtimeHooks = append(runtimeHooks, fn)
		return nil
	}

	runtimeMock := &ucmocks.MockRuntime{
		OnRuntimeBegunFn:           captureRuntime,
		OnRuntimeEndedFn:           captureRuntime,
		OnRuntimeRecoveredFn:       captureRuntime,
		OnRuntimeDetachedFn:        captureRuntime,
		OnRuntimePIDRecordedFn:     captureRuntime,
		OnRuntimeOutdatedFn:        captureRuntime,
		OnRuntimeOutdatedClearedFn: captureRuntime,
		OnRuntimeStepAdvancedFn:    captureRuntime,
	}
	collectionMock := &ucmocks.MockCollection{
		OnCollectionFollowedFn: func(fn func(context.Context, domain.Collection)) error {
			followedHook = fn
			return nil
		},
		OnCollectionUnfollowedFn: func(fn func(context.Context, domain.Namespace)) error {
			unfollowedHook = fn
			return nil
		},
	}

	c := &repositories.Container{Runtime: runtimeMock, Collection: collectionMock}
	hub := &stubHub{}
	require.NoError(t, c.RegisterHubProjections(hub))

	require.Len(t, runtimeHooks, 8)
	for _, fn := range runtimeHooks {
		fn(context.Background(), domainRuntime.ArrowRuntime{})
	}
	assert.Equal(t, int32(8), hub.runtimeBroadcasts.Load())

	require.NotNil(t, followedHook)
	require.NotNil(t, unfollowedHook)
	followedHook(context.Background(), domain.Collection{})
	unfollowedHook(context.Background(), domain.Namespace("github.com/user/q"))
	assert.Equal(t, int32(2), hub.quiverBroadcasts.Load())
}

// ─── New: construction failures ──────────────────────────────────────────────

func TestNew_GraphFails_ReturnsError(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	// arrow is constructed before graph, so a globally broken db (e.g. closed)
	// would surface as an arrow failure instead. A view squatting on graph's
	// table name leaves arrow's own tables untouched and fails only graph's
	// migration.
	require.NoError(t, db.Exec("CREATE VIEW graph_dep_edges AS SELECT 1 as from_namespace").Error)

	axArrow := newTestAsynxArrow(t)
	axRuntime := newTestAsynxRuntime(t)
	axCollection := newTestAsynxCollection(t)
	t.Cleanup(func() {
		_ = axArrow.Shutdown(context.Background())
		_ = axRuntime.Shutdown(context.Background())
		_ = axCollection.Shutdown(context.Background())
	})

	_, err = repositories.New(
		db, axArrow, axRuntime, axCollection, ":memory:",
		nil, nil, nil, domain.OSDarwinARM64, nil, nil,
		nil, nil, nil,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repositories: graph")
}

func TestNew_ArrowFails_ReturnsError(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)

	axArrow := &appmocks.AsynxArrow{
		SubscribeFn: func(
			_ string,
			_ asynxModels.ProjectionHandler[domain.Arrow],
			_ ...asynxModels.SubscriptionOpt[domain.Arrow],
		) (string, error) {
			return "", errors.New("subscribe boom")
		},
	}
	axRuntime := newTestAsynxRuntime(t)
	axCollection := newTestAsynxCollection(t)
	t.Cleanup(func() {
		_ = axRuntime.Shutdown(context.Background())
		_ = axCollection.Shutdown(context.Background())
	})

	_, err = repositories.New(
		db, axArrow, axRuntime, axCollection, ":memory:",
		nil, nil, nil, domain.OSDarwinARM64, nil, nil,
		nil, nil, nil,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repositories: arrow")
}

func TestNew_CollectionFails_ReturnsError(t *testing.T) {
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

	// A directory is not a database file.
	_, err = repositories.New(
		db, axArrow, axRuntime, axCollection, t.TempDir(),
		nil, nil, nil, domain.OSDarwinARM64, nil, nil,
		nil, nil, nil,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repositories: quiver")
}

func TestNew_PairingCodeFails_ReturnsError(t *testing.T) {
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

	_, err = repositories.New(
		db, axArrow, axRuntime, axCollection, ":memory:",
		nil, nil, nil, domain.OSDarwinARM64, nil, nil,
		nil, newTestAsynxDevice(t), db,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repositories: pairingcode")
}

func TestNew_DeviceFails_ReturnsError(t *testing.T) {
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

	_, err = repositories.New(
		db, axArrow, axRuntime, axCollection, ":memory:",
		nil, nil, nil, domain.OSDarwinARM64, nil, nil,
		newTestAsynxPairingCode(t), nil, db,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "repositories: device")
}

// ─── Adapters handed to the runtime ──────────────────────────────────────────

func TestArrowGetter_ReturnsAggregate(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1.0.0")
	want := domain.Arrow{Namespace: ns, ArrowMeta: domain.ArrowMeta{Name: "pkg"}}

	get := repositories.ArrowGetter(&appmocks.AsynxArrow{
		GetFn: func(_ context.Context, _ string) (domain.Arrow, error) {
			return want, nil
		},
	})

	got, err := get(context.Background(), ns)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, want, *got)
}

func TestArrowGetter_PropagatesError(t *testing.T) {
	boom := errors.New("not found")
	get := repositories.ArrowGetter(&appmocks.AsynxArrow{
		GetFn: func(_ context.Context, _ string) (domain.Arrow, error) {
			return domain.Arrow{}, boom
		},
	})

	_, err := get(context.Background(), domain.Namespace("github.com/user/pkg@v1.0.0"))
	assert.ErrorIs(t, err, boom)
}

func TestDependentsChecker_ExcludesNobody(t *testing.T) {
	var seenExclude domain.Namespace
	check := repositories.DependentsChecker(&ucmocks.MockGraph{
		HasDependentsFn: func(_ context.Context, _, exclude domain.Namespace) (bool, error) {
			seenExclude = exclude
			return true, nil
		},
	})

	has, err := check(context.Background(), domain.Namespace("github.com/user/pkg@v1.0.0"))
	require.NoError(t, err)
	assert.True(t, has)
	assert.Equal(t, domain.Namespace(""), seenExclude)
}

func TestCatalogLister_ListsEveryArrow(t *testing.T) {
	var seenFilter *bool
	list := repositories.CatalogLister(&ucmocks.MockArrow{
		ListFn: func(_ context.Context, userInstalled *bool) ([]models.ArrowView, error) {
			seenFilter = userInstalled
			return []models.ArrowView{{Namespace: "github.com/user/pkg"}}, nil
		},
	})

	got, err := list(context.Background())
	require.NoError(t, err)
	assert.Len(t, got, 1)
	assert.Nil(t, seenFilter, "the runtime wants the whole catalog, not just what a user asked for")
}

func TestDiscardCollection_LogsShutdownFailure(t *testing.T) {
	called := false
	repositories.DiscardCollection(&ucmocks.MockCollection{
		ShutdownFn: func(_ context.Context) error {
			called = true
			return errors.New("close failed")
		},
	})

	assert.True(t, called)
}
