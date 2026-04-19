package deps

import (
	"context"
	"fmt"
	"slices"

	"github.com/char2cs/asynx"
	depsstore "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/deps/store"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/manifest"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/deptree"
)

type Deps interface {
	Resolve(
		ctx context.Context,
		ns domain.Namespace,
	) (
		Plan,
		error,
	)

	Execute(
		ctx context.Context,
		plan Plan,
	) error

	Unplan(
		ctx context.Context,
		ns domain.Namespace,
	) (
		Plan,
		error,
	)

	HasDependents(
		ctx context.Context,
		ns domain.Namespace,
		excludeNs domain.Namespace,
	) (
		bool,
		error,
	)

	Orphans(
		ctx context.Context,
		ns domain.Namespace,
	) (
		[]domain.Namespace,
		error,
	)

	DiffDeps(
		old *domain.ArrowManifest,
		new *domain.ArrowManifest,
	) DepDiff
}

type Plan []PlanEntry

type PlanEntry struct {
	Namespace domain.Namespace
	Type      domain.DepType
}

type DepDiff struct {
	Added   []domain.DependencyEdge
	Removed []domain.DependencyEdge
}

type InstallSyncFunc func(
	ctx context.Context,
	ns domain.Namespace,
) error

type StartFunc func(
	ctx context.Context,
	ns domain.Namespace,
) error

type UninstallSyncFunc func(
	ctx context.Context,
	ns domain.Namespace,
) error

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

func New(
	axArrow asynx.Asynx[domain.Arrow],
	dt deptree.DepTree,
	resolve manifest.ResolveFunc,
	st depsstore.DepEdgeStore,
	install InstallSyncFunc,
	startFn StartFunc,
	uninstall UninstallSyncFunc,
) (Deps, error) {
	d := &depsService{
		depTree:         dt,
		resolveManifest: resolve,
		store:           st,
		fullStore:       st,
		installSync:     install,
		start:           startFn,
		uninstallSync:   uninstall,
	}

	if err := d.registerProjections(axArrow); err != nil {
		return nil, fmt.Errorf("deps: register projections: %w", err)
	}

	return d, nil
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

func (d *depsService) Execute(
	ctx context.Context,
	plan Plan,
) error {
	if len(plan) == 0 {
		return nil
	}

	var installed []PlanEntry

	for _, entry := range plan {
		if err := d.installSync(ctx, entry.Namespace); err != nil {
			d.rollback(ctx, installed)
			return err
		}

		installed = append(installed, entry)

		if entry.Type == domain.ServiceDep {
			_ = d.start(ctx, entry.Namespace)
		}
	}

	return nil
}

func (d *depsService) rollback(
	ctx context.Context,
	installed []PlanEntry,
) {
	for i := len(installed) - 1; i >= 0; i-- {
		_ = d.uninstallSync(ctx, installed[i].Namespace)
	}
}

func (d *depsService) HasDependents(
	ctx context.Context,
	ns domain.Namespace,
	excludeNs domain.Namespace,
) (bool, error) {
	return d.store.HasAnyDependents(
		ctx,
		ns.BareNamespace().String(),
		excludeNs.BareNamespace().String(),
	)
}

func (d *depsService) Orphans(
	ctx context.Context,
	ns domain.Namespace,
) ([]domain.Namespace, error) {
	plan, err := d.Resolve(ctx, ns)
	if err != nil {
		return nil, err
	}

	var orphans []domain.Namespace
	for _, entry := range plan {
		hasDeps, depErr := d.HasDependents(ctx, entry.Namespace, ns)
		if depErr != nil || hasDeps {
			continue
		}
		orphans = append(orphans, entry.Namespace.BareNamespace())
	}

	return orphans, nil
}
