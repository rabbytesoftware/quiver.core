package graph_test

import (
	"context"
	"errors"
	"testing"

	"github.com/char2cs/asynx"
	eventsqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	adapterSQLite "github.com/rabbytesoftware/quiver/internal/adapter/store/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/repositories/graph"
	"github.com/rabbytesoftware/quiver/internal/app/repositories/graph/internal/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── errStore: mock DepEdgeStore that returns errors ─────────────────────────

type errStore struct {
	saveErr             error
	hasDependentsErr    error
	edgesToBareErr      error
	hasDependentsResult bool
}

func (e *errStore) Save(_ context.Context, _, _ string, _ []store.DepEdgeRow) error {
	return e.saveErr
}
func (e *errStore) DeleteFrom(_ context.Context, _, _ string) error { return nil }
func (e *errStore) DeleteTo(_ context.Context, _ string) error      { return nil }
func (e *errStore) ByDependency(_ context.Context, _, _ string) ([]store.DepEdgeRow, error) {
	return nil, nil
}
func (e *errStore) EdgesToBare(_ context.Context, _ string) ([]store.DepEdgeRow, error) {
	return nil, e.edgesToBareErr
}
func (e *errStore) HasAnyDependents(_ context.Context, _, _ string) (bool, error) {
	return e.hasDependentsResult, e.hasDependentsErr
}

var testOS = domain.OSDarwinARM64

func newTestStore(t *testing.T) store.DepEdgeStore {
	t.Helper()
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	s, err := store.NewDepEdgeStore(db)
	require.NoError(t, err)
	return s
}

func newGraph(
	t *testing.T,
	edgeStore store.DepEdgeStore,
	resolveManifest func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error),
) graph.Graph {
	t.Helper()
	return graph.NewTestable(testOS, edgeStore, nil, resolveManifest)
}

func arrowWithTools(ns domain.Namespace, deps ...domain.Namespace) *domain.Arrow {
	var tools []domain.DependencyEdge
	for _, d := range deps {
		tools = append(tools, domain.DependencyEdge{Namespace: d})
	}
	return &domain.Arrow{
		Namespace: ns,
		Targets: map[domain.OS]domain.Target{
			testOS: {Tools: tools},
		},
	}
}

func arrowWithServices(ns domain.Namespace, deps ...domain.Namespace) *domain.Arrow {
	var services []domain.DependencyEdge
	for _, d := range deps {
		services = append(services, domain.DependencyEdge{Namespace: d})
	}
	return &domain.Arrow{
		Namespace: ns,
		Targets: map[domain.OS]domain.Target{
			testOS: {Services: services},
		},
	}
}

func TestResolve_NoDeps(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1")
	resolveManifest := func(_ context.Context, n domain.Namespace) (*domain.Arrow, error) {
		return &domain.Arrow{Namespace: n}, nil
	}

	g := newGraph(t, newTestStore(t), resolveManifest)
	plan, err := g.Resolve(context.Background(), ns)
	require.NoError(t, err)
	assert.Empty(t, plan)
}

func TestResolve_WithToolDep(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1")
	depNs := domain.Namespace("github.com/user/dep@v1")

	resolveManifest := func(_ context.Context, n domain.Namespace) (*domain.Arrow, error) {
		switch n.BareNamespace() {
		case ns.BareNamespace():
			return arrowWithTools(n, depNs), nil
		default:
			return &domain.Arrow{Namespace: n}, nil
		}
	}

	g := newGraph(t, newTestStore(t), resolveManifest)
	plan, err := g.Resolve(context.Background(), ns)
	require.NoError(t, err)
	require.Len(t, plan, 1)
	assert.Equal(t, domain.ToolDep, plan[0].Type)
}

func TestResolve_WithServiceDep(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1")
	depNs := domain.Namespace("github.com/user/svc@v1")

	resolveManifest := func(_ context.Context, n domain.Namespace) (*domain.Arrow, error) {
		switch n.BareNamespace() {
		case ns.BareNamespace():
			return arrowWithServices(n, depNs), nil
		default:
			return &domain.Arrow{Namespace: n}, nil
		}
	}

	g := newGraph(t, newTestStore(t), resolveManifest)
	plan, err := g.Resolve(context.Background(), ns)
	require.NoError(t, err)
	require.Len(t, plan, 1)
	assert.Equal(t, domain.ServiceDep, plan[0].Type)
}

func TestResolve_ManifestError(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1")
	resolveManifest := func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
		return nil, errors.New("manifest error")
	}

	g := newGraph(t, newTestStore(t), resolveManifest)
	_, err := g.Resolve(context.Background(), ns)
	require.Error(t, err)
}

func TestResolve_ConstraintExact_NilManifold(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1")
	depNs := domain.Namespace("github.com/user/dep")

	resolveManifest := func(_ context.Context, n domain.Namespace) (*domain.Arrow, error) {
		if n.BareNamespace() == ns.BareNamespace() {
			arrow := &domain.Arrow{
				Namespace: n,
				Targets: map[domain.OS]domain.Target{
					testOS: {Tools: []domain.DependencyEdge{{
						Namespace:  depNs,
						Constraint: "v1.0.0",
					}}},
				},
			}
			return arrow, nil
		}
		return &domain.Arrow{Namespace: n}, nil
	}

	// exact constraint (no glob) → bare+ref, no manifold needed
	g := graph.NewTestable(testOS, newTestStore(t), nil, resolveManifest)
	plan, err := g.Resolve(context.Background(), ns)
	require.NoError(t, err)
	require.Len(t, plan, 1)
	assert.Equal(t, "v1.0.0", plan[0].Namespace.Ref())
}

func TestResolve_ConstraintWithGlob_WithManifold(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1")
	depNs := domain.Namespace("github.com/user/dep")

	m := &mocks.Manifold{
		ResolveConstraintResult: "v1.2.3",
	}

	resolveManifest := func(_ context.Context, n domain.Namespace) (*domain.Arrow, error) {
		if n.BareNamespace() == ns.BareNamespace() {
			arrow := &domain.Arrow{
				Namespace: n,
				Targets: map[domain.OS]domain.Target{
					// Use "*" which is a real glob character
					testOS: {Tools: []domain.DependencyEdge{{
						Namespace:  depNs,
						Constraint: "v1.*",
					}}},
				},
			}
			return arrow, nil
		}
		return &domain.Arrow{Namespace: n}, nil
	}

	g := graph.NewTestable(testOS, newTestStore(t), m, resolveManifest)
	plan, err := g.Resolve(context.Background(), ns)
	require.NoError(t, err)
	require.Len(t, plan, 1)
	assert.Equal(t, "v1.2.3", plan[0].Namespace.Ref())
}

func TestUnplan_ReversesOrder(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1")
	depA := domain.Namespace("github.com/user/depA@v1")
	depB := domain.Namespace("github.com/user/depB@v1")

	resolveManifest := func(_ context.Context, n domain.Namespace) (*domain.Arrow, error) {
		switch n.BareNamespace() {
		case ns.BareNamespace():
			return arrowWithTools(n, depA, depB), nil
		default:
			return &domain.Arrow{Namespace: n}, nil
		}
	}

	g := newGraph(t, newTestStore(t), resolveManifest)
	plan, err := g.Resolve(context.Background(), ns)
	require.NoError(t, err)

	unplan, err := g.Unplan(context.Background(), ns)
	require.NoError(t, err)

	require.Len(t, unplan, len(plan))
	for i, j := 0, len(plan)-1; i <= j; i, j = i+1, j-1 {
		assert.Equal(t, plan[i].Namespace, unplan[j].Namespace)
		assert.Equal(t, plan[j].Namespace, unplan[i].Namespace)
	}
}

func TestHasDependents_True(t *testing.T) {
	es := newTestStore(t)
	// Seed a row manually: parent → dep
	err := es.Save(context.Background(), "github.com/user/parent", "v1", []store.DepEdgeRow{
		{
			FromNamespace: "github.com/user/parent",
			FromVersion:   "v1",
			ToNamespace:   "github.com/user/dep",
			ToVersion:     "v1",
			Constraint:    "",
			DepType:       "tool",
		},
	})
	require.NoError(t, err)

	g := newGraph(t, es, nil)
	has, err := g.HasDependents(context.Background(), domain.Namespace("github.com/user/dep@v1"), domain.Namespace(""))
	require.NoError(t, err)
	assert.True(t, has)
}

func TestHasDependents_False(t *testing.T) {
	g := newGraph(t, newTestStore(t), nil)
	has, err := g.HasDependents(context.Background(), domain.Namespace("github.com/user/nobody@v1"), domain.Namespace(""))
	require.NoError(t, err)
	assert.False(t, has)
}

func TestOrphans_NoOrphans(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1")
	depNs := domain.Namespace("github.com/user/dep@v1")

	es := newTestStore(t)
	// dep has another parent (not pkg) → not an orphan
	err := es.Save(context.Background(), "github.com/user/other", "v1", []store.DepEdgeRow{
		{
			FromNamespace: "github.com/user/other",
			FromVersion:   "v1",
			ToNamespace:   "github.com/user/dep",
			ToVersion:     "v1",
			Constraint:    "",
			DepType:       "tool",
		},
	})
	require.NoError(t, err)

	resolveManifest := func(_ context.Context, n domain.Namespace) (*domain.Arrow, error) {
		if n.BareNamespace() == ns.BareNamespace() {
			return arrowWithTools(n, depNs), nil
		}
		return &domain.Arrow{Namespace: n}, nil
	}

	g := newGraph(t, es, resolveManifest)
	orphans, err := g.Orphans(context.Background(), ns)
	require.NoError(t, err)
	assert.Empty(t, orphans)
}

func TestOrphans_WithOrphans(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1")
	depNs := domain.Namespace("github.com/user/dep@v1")

	resolveManifest := func(_ context.Context, n domain.Namespace) (*domain.Arrow, error) {
		if n.BareNamespace() == ns.BareNamespace() {
			return arrowWithTools(n, depNs), nil
		}
		return &domain.Arrow{Namespace: n}, nil
	}

	// No other parents for dep → orphan
	g := newGraph(t, newTestStore(t), resolveManifest)
	orphans, err := g.Orphans(context.Background(), ns)
	require.NoError(t, err)
	assert.Len(t, orphans, 1)
	assert.Equal(t, depNs.BareNamespace(), orphans[0].BareNamespace())
}

func TestGetDependents_ReturnsCorrectCallers(t *testing.T) {
	es := newTestStore(t)
	err := es.Save(context.Background(), "github.com/user/a", "v1", []store.DepEdgeRow{
		{
			FromNamespace: "github.com/user/a",
			FromVersion:   "v1",
			ToNamespace:   "github.com/user/b",
			ToVersion:     "v1",
			Constraint:    "",
			DepType:       "tool",
		},
	})
	require.NoError(t, err)

	g := newGraph(t, es, nil)
	dependents, err := g.GetDependents(context.Background(), domain.Namespace("github.com/user/b@v1"))
	require.NoError(t, err)
	require.Len(t, dependents, 1)
	assert.Equal(t, domain.Namespace("github.com/user/a").BareNamespace(), dependents[0].BareNamespace())
}

func TestGetDependents_Empty(t *testing.T) {
	g := newGraph(t, newTestStore(t), nil)
	dependents, err := g.GetDependents(context.Background(), domain.Namespace("github.com/user/nobody@v1"))
	require.NoError(t, err)
	assert.Empty(t, dependents)
}

func TestSyncDependencies_SetsEdges(t *testing.T) {
	es := newTestStore(t)
	ns := domain.Namespace("github.com/user/a@v1")
	depNs := domain.Namespace("github.com/user/b@v1")

	arrow := arrowWithTools(ns, depNs)
	g := newGraph(t, es, nil)

	err := g.SyncDependencies(context.Background(), ns, arrow)
	require.NoError(t, err)

	dependents, err := g.GetDependents(context.Background(), depNs)
	require.NoError(t, err)
	require.Len(t, dependents, 1)
	assert.Equal(t, ns.BareNamespace(), dependents[0].BareNamespace())
}

func TestRemoveDependencies_ClearsAllEdges(t *testing.T) {
	es := newTestStore(t)
	nsA := domain.Namespace("github.com/user/a@v1")
	nsB := domain.Namespace("github.com/user/b@v1")

	// A depends on B
	err := es.Save(context.Background(), "github.com/user/a", "v1", []store.DepEdgeRow{
		{
			FromNamespace: "github.com/user/a",
			FromVersion:   "v1",
			ToNamespace:   "github.com/user/b",
			ToVersion:     "v1",
			Constraint:    "",
			DepType:       "tool",
		},
	})
	require.NoError(t, err)

	g := newGraph(t, es, nil)
	err = g.RemoveDependencies(context.Background(), nsA)
	require.NoError(t, err)

	// A no longer appears as a dependent of B
	dependents, err := g.GetDependents(context.Background(), nsB)
	require.NoError(t, err)
	assert.Empty(t, dependents)
}

func TestDiffDeps_AddedRemoved(t *testing.T) {
	g := newGraph(t, newTestStore(t), nil)
	depA := domain.Namespace("github.com/user/depA@v1")
	depB := domain.Namespace("github.com/user/depB@v1")

	oldArrow := arrowWithTools(domain.Namespace("github.com/user/pkg@v1"), depA)
	newArrow := arrowWithTools(domain.Namespace("github.com/user/pkg@v1"), depB)

	diff := g.DiffDeps(oldArrow, newArrow)
	require.Len(t, diff.Added, 1)
	require.Len(t, diff.Removed, 1)
	assert.Equal(t, depB.BareNamespace(), diff.Added[0].Namespace.BareNamespace())
	assert.Equal(t, depA.BareNamespace(), diff.Removed[0].Namespace.BareNamespace())
}

func TestDiffDeps_ConstraintChanged(t *testing.T) {
	g := newGraph(t, newTestStore(t), nil)
	depNs := domain.Namespace("github.com/user/dep")

	oldArrow := &domain.Arrow{
		Namespace: domain.Namespace("github.com/user/pkg@v1"),
		Targets: map[domain.OS]domain.Target{
			testOS: {Tools: []domain.DependencyEdge{{Namespace: depNs, Constraint: "^v1"}}},
		},
	}
	newArrow := &domain.Arrow{
		Namespace: domain.Namespace("github.com/user/pkg@v1"),
		Targets: map[domain.OS]domain.Target{
			testOS: {Tools: []domain.DependencyEdge{{Namespace: depNs, Constraint: "^v2"}}},
		},
	}

	diff := g.DiffDeps(oldArrow, newArrow)
	assert.Empty(t, diff.Added)
	assert.Empty(t, diff.Removed)
	require.Len(t, diff.Constrained, 1)
	assert.Equal(t, "^v1", diff.Constrained[0].OldConstraint)
	assert.Equal(t, "^v2", diff.Constrained[0].NewConstraint)
}

func TestDiffDeps_NilArrows(t *testing.T) {
	g := newGraph(t, newTestStore(t), nil)
	diff := g.DiffDeps(nil, nil)
	assert.Empty(t, diff.Added)
	assert.Empty(t, diff.Removed)
	assert.Empty(t, diff.Constrained)
}

// ─── New ─────────────────────────────────────────────────────────────────────

func newTestAsynxArrow(t *testing.T) asynx.Asynx[domain.Arrow] {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Arrow]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 4, QueueDepth: 100}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return ax
}

func TestNew_Success(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	axArrow := newTestAsynxArrow(t)

	g, err := graph.New(db, axArrow, testOS, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, g)
}

func TestNew_StoreError(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	axArrow := newTestAsynxArrow(t)
	_, err = graph.New(db, axArrow, testOS, nil, nil)
	require.Error(t, err)
}

func TestNew_RegisterError(t *testing.T) {
	db, err := adapterSQLite.OpenDB(":memory:")
	require.NoError(t, err)
	axArrow := newTestAsynxArrow(t)
	// Shut down before New so Subscribe fails.
	require.NoError(t, axArrow.Shutdown(context.Background()))

	_, err = graph.New(db, axArrow, testOS, nil, nil)
	require.Error(t, err)
}

// ─── Unplan: resolve error ────────────────────────────────────────────────────

func TestUnplan_ResolveError(t *testing.T) {
	resolveManifest := func(_ context.Context, _ domain.Namespace) (*domain.Arrow, error) {
		return nil, errors.New("manifest error")
	}
	g := newGraph(t, newTestStore(t), resolveManifest)
	_, err := g.Unplan(context.Background(), domain.Namespace("github.com/user/pkg@v1"))
	require.Error(t, err)
}

// ─── Orphans: HasDependents error ────────────────────────────────────────────

func TestOrphans_HasDependentsError(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1")
	depNs := domain.Namespace("github.com/user/dep@v1")

	resolveManifest := func(_ context.Context, n domain.Namespace) (*domain.Arrow, error) {
		if n.BareNamespace() == ns.BareNamespace() {
			return arrowWithTools(n, depNs), nil
		}
		return &domain.Arrow{Namespace: n}, nil
	}

	es := &errStore{hasDependentsErr: errors.New("db error")}
	g := graph.NewTestable(testOS, es, nil, resolveManifest)
	_, err := g.Orphans(context.Background(), ns)
	require.Error(t, err)
}

// ─── GetDependents: EdgesToBare error ────────────────────────────────────────

func TestGetDependents_EdgesToBareError(t *testing.T) {
	es := &errStore{edgesToBareErr: errors.New("db error")}
	g := graph.NewTestable(testOS, es, nil, nil)
	_, err := g.GetDependents(
		context.Background(),
		domain.Namespace("github.com/user/dep@v1"),
	)
	require.Error(t, err)
}

// ─── resolveEdgeNs: manifold ResolveConstraint error ─────────────────────────

func TestResolve_ConstraintWithGlob_ManifoldError(t *testing.T) {
	ns := domain.Namespace("github.com/user/pkg@v1")
	depNs := domain.Namespace("github.com/user/dep")

	m := &mocks.Manifold{
		ResolveConstraintErr: errors.New("constraint resolve error"),
	}

	resolveManifest := func(_ context.Context, n domain.Namespace) (*domain.Arrow, error) {
		if n.BareNamespace() == ns.BareNamespace() {
			return &domain.Arrow{
				Namespace: n,
				Targets: map[domain.OS]domain.Target{
					testOS: {Tools: []domain.DependencyEdge{{
						Namespace:  depNs,
						Constraint: "v1.*",
					}}},
				},
			}, nil
		}
		return &domain.Arrow{Namespace: n}, nil
	}

	g := graph.NewTestable(testOS, newTestStore(t), m, resolveManifest)
	_, err := g.Resolve(context.Background(), ns)
	require.Error(t, err)
}
