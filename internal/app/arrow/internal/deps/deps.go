package deps

import (
	"context"
	"fmt"

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
