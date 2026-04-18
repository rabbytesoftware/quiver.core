package graph

import (
	"context"
	"testing"
	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	sqlite "github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/commands"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/graph/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newAxArrow(t *testing.T) asynx.Asynx[domain.Arrow] {
	t.Helper()
	es, err := sqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Arrow]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 4, QueueDepth: 100}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { ax.Shutdown(context.Background()) })
	return ax
}

func newGraph(t *testing.T, ax asynx.Asynx[domain.Arrow]) Graph {
	t.Helper()
	st, err := store.NewDepEdgeStore(":memory:")
	require.NoError(t, err)
	g, err := New(ax, st)
	require.NoError(t, err)
	return g
}

func depEdge(ns, constraint string, dt domain.DepType) domain.DependencyEdge {
	return domain.DependencyEdge{
		Namespace:  domain.Namespace(ns),
		Constraint: constraint,
		Type:       dt,
	}
}

// --- SaveEdges / DeleteEdges ---

func TestSaveEdges_empty(t *testing.T) {
	ax := newAxArrow(t)
	g := newGraph(t, ax)
	ctx := context.Background()

	fromNs := domain.Namespace("github.com/org/app")
	err := g.SaveEdges(ctx, fromNs, "1.0.0", nil)
	assert.NoError(t, err)
}

func TestSaveEdges_and_Dependents(t *testing.T) {
	ax := newAxArrow(t)
	g := newGraph(t, ax)
	ctx := context.Background()

	fromNs := domain.Namespace("github.com/org/app")
	toNs := domain.Namespace("github.com/org/lib")

	edges := []domain.DependencyEdge{
		depEdge("github.com/org/lib@1.0.0", "1.*", domain.ToolDep),
	}
	require.NoError(t, g.SaveEdges(ctx, fromNs, "2.0.0", edges))

	deps, err := g.Dependents(ctx, toNs, "1.0.0")
	require.NoError(t, err)
	require.Len(t, deps, 1)
	assert.Equal(t, domain.Namespace("github.com/org/app@2.0.0"), deps[0].Namespace)
	assert.Equal(t, "1.*", deps[0].Constraint)
	assert.Equal(t, domain.ToolDep, deps[0].Type)
}

func TestSaveEdges_idempotent(t *testing.T) {
	ax := newAxArrow(t)
	g := newGraph(t, ax)
	ctx := context.Background()

	fromNs := domain.Namespace("github.com/org/app")
	toNs := domain.Namespace("github.com/org/lib")

	edges := []domain.DependencyEdge{
		depEdge("github.com/org/lib@1.0.0", "1.*", domain.ToolDep),
	}

	require.NoError(t, g.SaveEdges(ctx, fromNs, "2.0.0", edges))
	require.NoError(t, g.SaveEdges(ctx, fromNs, "2.0.0", edges))

	deps, err := g.Dependents(ctx, toNs, "1.0.0")
	require.NoError(t, err)
	assert.Len(t, deps, 1)
}

func TestDeleteEdges(t *testing.T) {
	ax := newAxArrow(t)
	g := newGraph(t, ax)
	ctx := context.Background()

	fromNs := domain.Namespace("github.com/org/app")
	toNs := domain.Namespace("github.com/org/lib")

	edges := []domain.DependencyEdge{
		depEdge("github.com/org/lib@1.0.0", "1.*", domain.ToolDep),
	}
	require.NoError(t, g.SaveEdges(ctx, fromNs, "2.0.0", edges))

	require.NoError(t, g.DeleteEdges(ctx, fromNs, "2.0.0"))

	deps, err := g.Dependents(ctx, toNs, "1.0.0")
	require.NoError(t, err)
	assert.Empty(t, deps)
}

// --- HasDependents ---

func TestHasDependents_no_dependents(t *testing.T) {
	ax := newAxArrow(t)
	g := newGraph(t, ax)
	ctx := context.Background()

	ns := domain.Namespace("github.com/org/lib")
	excludeNs := domain.Namespace("github.com/org/app")

	has, err := g.HasDependents(ctx, ns, excludeNs)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestHasDependents_with_dependent(t *testing.T) {
	ax := newAxArrow(t)
	g := newGraph(t, ax)
	ctx := context.Background()

	fromNs := domain.Namespace("github.com/org/app")
	toNs := domain.Namespace("github.com/org/lib")
	excludeNs := domain.Namespace("github.com/org/other")

	edges := []domain.DependencyEdge{
		depEdge("github.com/org/lib@1.0.0", "1.*", domain.ToolDep),
	}
	require.NoError(t, g.SaveEdges(ctx, fromNs, "2.0.0", edges))

	has, err := g.HasDependents(ctx, toNs, excludeNs)
	require.NoError(t, err)
	assert.True(t, has)
}

func TestHasDependents_excludes_self(t *testing.T) {
	ax := newAxArrow(t)
	g := newGraph(t, ax)
	ctx := context.Background()

	fromNs := domain.Namespace("github.com/org/app")
	toNs := domain.Namespace("github.com/org/lib")

	edges := []domain.DependencyEdge{
		depEdge("github.com/org/lib@1.0.0", "1.*", domain.ToolDep),
	}
	require.NoError(t, g.SaveEdges(ctx, fromNs, "2.0.0", edges))

	// exclude fromNs — so the one dependent is excluded, result should be false
	has, err := g.HasDependents(ctx, toNs, fromNs)
	require.NoError(t, err)
	assert.False(t, has)
}

// --- CanUpgrade ---

func TestCanUpgrade_matching_constraint(t *testing.T) {
	ax := newAxArrow(t)
	g := newGraph(t, ax)
	ctx := context.Background()

	fromNs := domain.Namespace("github.com/org/app")
	toNs := domain.Namespace("github.com/org/lib")

	edges := []domain.DependencyEdge{
		depEdge("github.com/org/lib@1.0.0", "1.*", domain.ToolDep),
	}
	require.NoError(t, g.SaveEdges(ctx, fromNs, "2.0.0", edges))

	compatible, err := g.CanUpgrade(ctx, toNs, "1.0.0", "1.2.0")
	require.NoError(t, err)
	require.Len(t, compatible, 1)
	assert.Equal(t, domain.Namespace("github.com/org/app@2.0.0"), compatible[0].Namespace)
}

func TestCanUpgrade_non_matching_constraint(t *testing.T) {
	ax := newAxArrow(t)
	g := newGraph(t, ax)
	ctx := context.Background()

	fromNs := domain.Namespace("github.com/org/app")
	toNs := domain.Namespace("github.com/org/lib")

	edges := []domain.DependencyEdge{
		depEdge("github.com/org/lib@1.0.0", "1.*", domain.ToolDep),
	}
	require.NoError(t, g.SaveEdges(ctx, fromNs, "2.0.0", edges))

	compatible, err := g.CanUpgrade(ctx, toNs, "1.0.0", "2.0.0")
	require.NoError(t, err)
	assert.Empty(t, compatible)
}

func TestCanUpgrade_no_edges(t *testing.T) {
	ax := newAxArrow(t)
	g := newGraph(t, ax)
	ctx := context.Background()

	toNs := domain.Namespace("github.com/org/lib")
	compatible, err := g.CanUpgrade(ctx, toNs, "1.0.0", "1.2.0")
	require.NoError(t, err)
	assert.Empty(t, compatible)
}

// --- Projections ---

func sendAndWait(t *testing.T, ax asynx.Asynx[domain.Arrow], cmd asynxModels.Command[domain.Arrow]) {
	t.Helper()
	_, err := ax.SendWait(context.Background(), cmd)
	require.NoError(t, err)
	ax.WaitPublish()
}

func TestProjection_arrowAdded_savesEdges(t *testing.T) {
	ax := newAxArrow(t)
	g := newGraph(t, ax)
	ctx := context.Background()

	fromNs := domain.Namespace("github.com/org/app")
	toNs := domain.Namespace("github.com/org/lib")
	manifest := domain.ArrowManifest{
		ArrowMeta: domain.ArrowMeta{Name: "app", Version: "1.0.0"},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Tools: []domain.DependencyEdge{
					depEdge("github.com/org/lib@1.0.0", "1.*", domain.ToolDep),
				},
			},
		},
	}

	sendAndWait(t, ax, commands.AddArrow{Namespace: fromNs, Manifest: manifest})

	deps, err := g.Dependents(ctx, toNs, "1.0.0")
	require.NoError(t, err)
	require.Len(t, deps, 1)
	assert.Equal(t, domain.Namespace("github.com/org/app@1.0.0"), deps[0].Namespace)
}

func TestProjection_arrowUpdated_replacesEdges(t *testing.T) {
	ax := newAxArrow(t)
	g := newGraph(t, ax)
	ctx := context.Background()

	fromNs := domain.Namespace("github.com/org/app")
	toNsOld := domain.Namespace("github.com/org/libold")
	toNsNew := domain.Namespace("github.com/org/libnew")

	manifest1 := domain.ArrowManifest{
		ArrowMeta: domain.ArrowMeta{Name: "app", Version: "1.0.0"},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Tools: []domain.DependencyEdge{
					depEdge("github.com/org/libold@1.0.0", "1.*", domain.ToolDep),
				},
			},
		},
	}
	manifest2 := domain.ArrowManifest{
		ArrowMeta: domain.ArrowMeta{Name: "app", Version: "1.0.0"},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Tools: []domain.DependencyEdge{
					depEdge("github.com/org/libnew@2.0.0", "2.*", domain.ToolDep),
				},
			},
		},
	}

	sendAndWait(t, ax, commands.AddArrow{Namespace: fromNs, Manifest: manifest1})
	sendAndWait(t, ax, commands.UpdateArrowManifest{Namespace: fromNs, Manifest: manifest2})

	depsOld, err := g.Dependents(ctx, toNsOld, "1.0.0")
	require.NoError(t, err)
	assert.Empty(t, depsOld)

	depsNew, err := g.Dependents(ctx, toNsNew, "2.0.0")
	require.NoError(t, err)
	assert.Len(t, depsNew, 1)
}

func TestProjection_arrowRemoved_deletesEdges(t *testing.T) {
	ax := newAxArrow(t)
	g := newGraph(t, ax)
	ctx := context.Background()

	fromNs := domain.Namespace("github.com/org/app")
	toNs := domain.Namespace("github.com/org/lib")
	manifest := domain.ArrowManifest{
		ArrowMeta: domain.ArrowMeta{Name: "app", Version: "1.0.0"},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Tools: []domain.DependencyEdge{
					depEdge("github.com/org/lib@1.0.0", "1.*", domain.ToolDep),
				},
			},
		},
	}

	sendAndWait(t, ax, commands.AddArrow{Namespace: fromNs, Manifest: manifest})

	deps, err := g.Dependents(ctx, toNs, "1.0.0")
	require.NoError(t, err)
	require.Len(t, deps, 1)

	err = ax.Forget(ctx, fromNs.String())
	require.NoError(t, err)
	ax.WaitPublish()

	deps, err = g.Dependents(ctx, toNs, "1.0.0")
	require.NoError(t, err)
	assert.Empty(t, deps)
}

func TestProjection_deduplicatesAcrossTargets(t *testing.T) {
	ax := newAxArrow(t)
	g := newGraph(t, ax)
	ctx := context.Background()

	fromNs := domain.Namespace("github.com/org/app")
	toNs := domain.Namespace("github.com/org/lib")

	// Same dep appears in two OS targets — should only be stored once
	manifest := domain.ArrowManifest{
		ArrowMeta: domain.ArrowMeta{Name: "app", Version: "1.0.0"},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Tools: []domain.DependencyEdge{
					depEdge("github.com/org/lib@1.0.0", "1.*", domain.ToolDep),
				},
			},
			domain.OSDarwinAMD64: {
				Tools: []domain.DependencyEdge{
					depEdge("github.com/org/lib@1.0.0", "1.*", domain.ToolDep),
				},
			},
		},
	}

	sendAndWait(t, ax, commands.AddArrow{Namespace: fromNs, Manifest: manifest})

	deps, err := g.Dependents(ctx, toNs, "1.0.0")
	require.NoError(t, err)
	assert.Len(t, deps, 1)
}

func TestDependents_multipleVersions(t *testing.T) {
	ax := newAxArrow(t)
	g := newGraph(t, ax)
	ctx := context.Background()

	toNs := domain.Namespace("github.com/org/lib")
	fromNs1 := domain.Namespace("github.com/org/app1")
	fromNs2 := domain.Namespace("github.com/org/app2")

	require.NoError(t, g.SaveEdges(ctx, fromNs1, "1.0.0", []domain.DependencyEdge{
		depEdge("github.com/org/lib@2.0.0", "2.*", domain.ToolDep),
	}))
	require.NoError(t, g.SaveEdges(ctx, fromNs2, "3.0.0", []domain.DependencyEdge{
		depEdge("github.com/org/lib@2.0.0", "2.*", domain.ServiceDep),
	}))

	deps, err := g.Dependents(ctx, toNs, "2.0.0")
	require.NoError(t, err)
	assert.Len(t, deps, 2)
}

func TestSaveEdges_serviceDepType(t *testing.T) {
	ax := newAxArrow(t)
	g := newGraph(t, ax)
	ctx := context.Background()

	fromNs := domain.Namespace("github.com/org/app")
	toNs := domain.Namespace("github.com/org/svc")

	edges := []domain.DependencyEdge{
		depEdge("github.com/org/svc@3.0.0", "3.*", domain.ServiceDep),
	}
	require.NoError(t, g.SaveEdges(ctx, fromNs, "1.0.0", edges))

	deps, err := g.Dependents(ctx, toNs, "3.0.0")
	require.NoError(t, err)
	require.Len(t, deps, 1)
	assert.Equal(t, domain.ServiceDep, deps[0].Type)
}

func TestProjection_serviceEdgesFromTargets(t *testing.T) {
	ax := newAxArrow(t)
	g := newGraph(t, ax)
	ctx := context.Background()

	fromNs := domain.Namespace("github.com/org/app")
	toNs := domain.Namespace("github.com/org/svc")

	manifest := domain.ArrowManifest{
		ArrowMeta: domain.ArrowMeta{Name: "app", Version: "1.0.0"},
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Services: []domain.DependencyEdge{
					depEdge("github.com/org/svc@1.0.0", "1.*", domain.ServiceDep),
				},
			},
		},
	}

	sendAndWait(t, ax, commands.AddArrow{Namespace: fromNs, Manifest: manifest})

	deps, err := g.Dependents(ctx, toNs, "1.0.0")
	require.NoError(t, err)
	require.Len(t, deps, 1)
	assert.Equal(t, domain.ServiceDep, deps[0].Type)
}

func TestCanUpgrade_serviceDepCompatible(t *testing.T) {
	ax := newAxArrow(t)
	g := newGraph(t, ax)
	ctx := context.Background()

	fromNs := domain.Namespace("github.com/org/app")
	toNs := domain.Namespace("github.com/org/svc")

	edges := []domain.DependencyEdge{
		depEdge("github.com/org/svc@1.0.0", "1.*", domain.ServiceDep),
	}
	require.NoError(t, g.SaveEdges(ctx, fromNs, "2.0.0", edges))

	compatible, err := g.CanUpgrade(ctx, toNs, "1.0.0", "1.5.0")
	require.NoError(t, err)
	require.Len(t, compatible, 1)
	assert.Equal(t, domain.ServiceDep, compatible[0].Type)
}

func TestDependents_empty(t *testing.T) {
	ax := newAxArrow(t)
	g := newGraph(t, ax)
	ctx := context.Background()

	toNs := domain.Namespace("github.com/org/lib")
	deps, err := g.Dependents(ctx, toNs, "1.0.0")
	require.NoError(t, err)
	assert.Empty(t, deps)
}
