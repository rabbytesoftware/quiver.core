package deps

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
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
