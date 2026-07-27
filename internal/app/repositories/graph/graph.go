package graph

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	graphinternal "github.com/rabbytesoftware/quiver.core/internal/app/repositories/graph/internal"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/graph/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/deptree"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
)

type (
	Plan           = models.Plan
	PlanEntry      = models.PlanEntry
	ConstrainedDep = models.ConstrainedDep
	DepDiff        = models.DepDiff
)

type Graph interface {
	Resolve(
		ctx context.Context,
		ns domain.Namespace,
	) (Plan, error)

	Unplan(
		ctx context.Context,
		ns domain.Namespace,
	) (Plan, error)

	HasDependents(
		ctx context.Context,
		ns domain.Namespace,
		excludeNs domain.Namespace,
	) (bool, error)

	Orphans(
		ctx context.Context,
		ns domain.Namespace,
	) ([]domain.Namespace, error)

	GetDependents(
		ctx context.Context,
		ns domain.Namespace,
	) ([]domain.Namespace, error)

	SyncDependencies(
		ctx context.Context,
		ns domain.Namespace,
		arrow *domain.Arrow,
	) error

	RemoveDependencies(
		ctx context.Context,
		ns domain.Namespace,
	) error

	DiffDeps(
		old, new *domain.Arrow,
	) DepDiff
}

type graphService struct {
	os              domain.OS
	edgeStore       store.DepEdgeStore
	manifoldSvc     manifold.Manifold
	resolveManifest func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error)
	depTree         deptree.DepTree
}

// New builds the dependency graph over db. resolveManifest is injected so graph
// does not depend on vault or manifold.
//
// graph subscribes to nothing. It used to keep its own arrow.added projection
// alongside the SyncDependencies the container wires, which left two
// unsequenced writers racing for the same edge rows with different contents.
// The container callback is now the only writer.
func New(
	db *gorm.DB,
	os domain.OS,
	manifoldSvc manifold.Manifold,
	resolveManifest func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error),
) (Graph, error) {
	edgeStore, err := store.NewDepEdgeStore(db)
	if err != nil {
		return nil, fmt.Errorf("graph: init store: %w", err)
	}

	return &graphService{
		os:              os,
		edgeStore:       edgeStore,
		manifoldSvc:     manifoldSvc,
		resolveManifest: resolveManifest,
		depTree:         deptree.New(),
	}, nil
}

func NewTestable(
	os domain.OS,
	edgeStore store.DepEdgeStore,
	manifoldSvc manifold.Manifold,
	resolveManifest func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error),
) Graph {
	return &graphService{
		os:              os,
		edgeStore:       edgeStore,
		manifoldSvc:     manifoldSvc,
		resolveManifest: resolveManifest,
		depTree:         deptree.New(),
	}
}

func (g *graphService) Resolve(
	ctx context.Context,
	ns domain.Namespace,
) (Plan, error) {
	typeIndex := make(map[domain.Namespace]domain.DepType)

	resolver := func(
		ctx context.Context,
		depNs domain.Namespace,
	) ([]domain.Namespace, error) {
		m, err := g.resolveManifest(ctx, depNs)
		if err != nil {
			return nil, err
		}

		target, ok := m.Targets[g.os]
		if !ok {
			return nil, nil
		}

		var children []domain.Namespace
		for _, edge := range target.Tools {
			resolved, resolveErr := g.resolveEdgeNs(ctx, edge)
			if resolveErr != nil {
				return nil, resolveErr
			}
			typeIndex[resolved.BareNamespace()] = domain.ToolDep
			children = append(children, resolved)
		}

		for _, edge := range target.Services {
			resolved, resolveErr := g.resolveEdgeNs(ctx, edge)
			if resolveErr != nil {
				return nil, resolveErr
			}
			typeIndex[resolved.BareNamespace()] = domain.ServiceDep
			children = append(children, resolved)
		}

		return graphinternal.DedupNamespaces(children), nil
	}

	ordered, err := g.depTree.Resolve(ctx, ns, resolver)
	if err != nil {
		return nil, err
	}

	rootBare := ns.BareNamespace()
	var plan Plan
	for _, dep := range ordered {
		if dep.BareNamespace() == rootBare {
			continue
		}
		depType, ok := typeIndex[dep.BareNamespace()]
		if !ok {
			depType = domain.ToolDep
		}
		plan = append(plan, PlanEntry{
			Namespace: dep,
			Type:      depType,
		})
	}

	return plan, nil
}

func (g *graphService) Unplan(
	ctx context.Context,
	ns domain.Namespace,
) (Plan, error) {
	plan, err := g.Resolve(ctx, ns)
	if err != nil {
		return nil, err
	}

	result := make(Plan, len(plan))
	copy(result, plan)
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return result, nil
}

func (g *graphService) HasDependents(
	ctx context.Context,
	ns domain.Namespace,
	excludeNs domain.Namespace,
) (bool, error) {
	return g.edgeStore.HasAnyDependents(
		ctx,
		ns.BareNamespace().String(),
		excludeNs.BareNamespace().String(),
	)
}

func (g *graphService) Orphans(
	ctx context.Context,
	ns domain.Namespace,
) ([]domain.Namespace, error) {
	plan, err := g.Resolve(ctx, ns)
	if err != nil {
		return nil, err
	}

	var orphans []domain.Namespace
	for _, entry := range plan {
		hasDeps, depErr := g.HasDependents(ctx, entry.Namespace, ns)
		if depErr != nil {
			return nil, fmt.Errorf("orphans: check dependents for %s: %w", entry.Namespace, depErr)
		}
		if hasDeps {
			continue
		}
		orphans = append(orphans, entry.Namespace)
	}

	return orphans, nil
}

func (g *graphService) GetDependents(
	ctx context.Context,
	ns domain.Namespace,
) ([]domain.Namespace, error) {
	rows, err := g.edgeStore.EdgesToBare(ctx, ns.BareNamespace().String())
	if err != nil {
		return nil, err
	}

	var dependents []domain.Namespace
	for _, row := range rows {
		dependents = append(dependents, domain.Namespace(row.FromNamespace).WithRef(row.FromVersion))
	}

	return dependents, nil
}

func (g *graphService) SyncDependencies(
	ctx context.Context,
	ns domain.Namespace,
	arrow *domain.Arrow,
) error {
	edges := graphinternal.CollectEdgesForOS(arrow, g.os)

	rows := make([]store.DepEdgeRow, 0, len(edges))
	for _, e := range edges {
		rows = append(rows, store.DepEdgeRow{
			FromNamespace: ns.BareNamespace().String(),
			FromVersion:   ns.Ref(),
			ToNamespace:   e.Namespace.BareNamespace().String(),
			ToVersion:     e.Namespace.Ref(),
			Constraint:    e.Constraint,
			DepType:       string(e.Type),
		})
	}

	return g.edgeStore.Save(ctx, ns.BareNamespace().String(), ns.Ref(), rows)
}

func (g *graphService) RemoveDependencies(
	ctx context.Context,
	ns domain.Namespace,
) error {
	if err := g.edgeStore.DeleteFrom(ctx, ns.BareNamespace().String(), ns.Ref()); err != nil {
		return err
	}
	return g.edgeStore.DeleteTo(ctx, ns.BareNamespace().String())
}

func (g *graphService) DiffDeps(
	old *domain.Arrow,
	new *domain.Arrow,
) DepDiff {
	oldEdges := graphinternal.CollectEdgesForOS(old, g.os)
	newEdges := graphinternal.CollectEdgesForOS(new, g.os)

	oldByBare := make(map[domain.Namespace]domain.DependencyEdge, len(oldEdges))
	for _, e := range oldEdges {
		oldByBare[e.Namespace.BareNamespace()] = e
	}

	newByBare := make(map[domain.Namespace]domain.DependencyEdge, len(newEdges))
	for _, e := range newEdges {
		newByBare[e.Namespace.BareNamespace()] = e
	}

	var added []domain.DependencyEdge
	for bare, e := range newByBare {
		if _, exists := oldByBare[bare]; !exists {
			added = append(added, e)
		}
	}

	var removed []domain.DependencyEdge
	for bare, e := range oldByBare {
		if _, exists := newByBare[bare]; !exists {
			removed = append(removed, e)
		}
	}

	var constrained []ConstrainedDep
	for bare, oldEdge := range oldByBare {
		newEdge, exists := newByBare[bare]
		if !exists {
			continue
		}
		if oldEdge.Constraint == newEdge.Constraint {
			continue
		}
		constrained = append(constrained, ConstrainedDep{
			Namespace:     bare,
			OldConstraint: oldEdge.Constraint,
			NewConstraint: newEdge.Constraint,
		})
	}

	return DepDiff{
		Added:       added,
		Removed:     removed,
		Constrained: constrained,
	}
}

func (g *graphService) resolveEdgeNs(
	ctx context.Context,
	edge domain.DependencyEdge,
) (domain.Namespace, error) {
	ref := edge.Constraint
	if ref == "" {
		return edge.Namespace.BareNamespace(), nil
	}
	if containsGlob(ref) { //nolint:nestif
		if g.manifoldSvc == nil {
			return edge.Namespace.BareNamespace().WithRef(ref), nil
		}
		resolved, err := g.manifoldSvc.ResolveConstraint(ctx, edge.Namespace, ref)
		if err != nil {
			return "", fmt.Errorf("graph: resolve constraint %q for %s: %w",
				ref, edge.Namespace, err)
		}
		return edge.Namespace.BareNamespace().WithRef(resolved), nil
	}
	return edge.Namespace.BareNamespace().WithRef(ref), nil
}

func containsGlob(s string) bool {
	return strings.ContainsAny(s, "*?[")
}
