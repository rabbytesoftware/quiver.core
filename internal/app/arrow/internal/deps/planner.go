package deps

import (
	"context"
	"slices"

	depsstore "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/deps/store"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/manifest"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
)

type depEdgeStoreInternal interface {
	HasAnyDependents(
		ctx context.Context,
		toNs, excludeFromNs string,
	) (bool, error)

	ByDependency(
		ctx context.Context,
		toNs, toVersion string,
	) ([]depsstore.DepEdgeRow, error)
}

type depsService struct {
	depTree         deptree.DepTree
	resolveManifest manifest.ResolveFunc
	store           depEdgeStoreInternal
	fullStore       depsstore.DepEdgeStore
	installSync     InstallSyncFunc
	start           StartFunc
	uninstallSync   UninstallSyncFunc
}

func NewTestable(
	dt deptree.DepTree,
	resolve manifest.ResolveFunc,
	st depEdgeStoreInternal,
	install InstallSyncFunc,
	startFn StartFunc,
	uninstall UninstallSyncFunc,
) Deps {
	return &depsService{
		depTree:         dt,
		resolveManifest: resolve,
		store:           st,
		fullStore:       nil,
		installSync:     install,
		start:           startFn,
		uninstallSync:   uninstall,
	}
}

func (d *depsService) Resolve(
	ctx context.Context,
	ns domain.Namespace,
) (Plan, error) {
	typeIndex := make(map[domain.Namespace]domain.DepType)

	resolver := func(
		ctx context.Context,
		depNs domain.Namespace,
	) ([]domain.Namespace, error) {
		m, err := d.resolveManifest(ctx, depNs)
		if err != nil {
			return nil, err
		}

		var children []domain.Namespace
		for _, target := range m.Targets {
			for _, edge := range target.Tools {
				bare := edge.Namespace.BareNamespace()
				typeIndex[bare] = domain.ToolDep
				children = append(children, bare)
			}
			for _, edge := range target.Services {
				bare := edge.Namespace.BareNamespace()
				typeIndex[bare] = domain.ServiceDep
				children = append(children, bare)
			}
		}

		return dedupNamespaces(children), nil
	}

	ordered, err := d.depTree.Resolve(ctx, ns, resolver)
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

func (d *depsService) Unplan(
	ctx context.Context,
	ns domain.Namespace,
) (Plan, error) {
	plan, err := d.Resolve(ctx, ns)
	if err != nil {
		return nil, err
	}

	slices.Reverse(plan)

	return plan, nil
}

func (d *depsService) DiffDeps(
	old *domain.ArrowManifest,
	new *domain.ArrowManifest,
) DepDiff {
	oldEdges := collectEdges(old)
	newEdges := collectEdges(new)

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

	return DepDiff{
		Added:   added,
		Removed: removed,
	}
}

func collectEdges(
	m *domain.ArrowManifest,
) []domain.DependencyEdge {
	if m == nil {
		return nil
	}

	seen := make(map[domain.Namespace]struct{})
	var edges []domain.DependencyEdge

	for _, target := range m.Targets {
		for _, e := range target.Tools {
			bare := e.Namespace.BareNamespace()
			if _, ok := seen[bare]; ok {
				continue
			}
			seen[bare] = struct{}{}
			edges = append(edges, e)
		}
		for _, e := range target.Services {
			bare := e.Namespace.BareNamespace()
			if _, ok := seen[bare]; ok {
				continue
			}
			seen[bare] = struct{}{}
			edges = append(edges, e)
		}
	}

	return edges
}

func dedupNamespaces(
	ns []domain.Namespace,
) []domain.Namespace {
	seen := make(map[domain.Namespace]struct{}, len(ns))
	var result []domain.Namespace
	for _, n := range ns {
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		result = append(result, n)
	}
	return result
}
