package bracket

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

type Arrow interface {
	ResolveCatalogued(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.Namespace, error)
	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
	CheckAvailable(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Available, error)
	RefreshToTarget(
		ctx context.Context,
		ns domain.Namespace,
		target domain.Available,
	) (*domain.Arrow, error)
	Advance(
		ctx context.Context,
		ns domain.Namespace,
		target domain.Available,
	) error
}

type Runtime interface {
	GetState(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.ArrowState, error)
	GetRuntime(
		ctx context.Context,
		ns domain.Namespace,
	) (*domainRuntime.ArrowRuntime, error)
	BeginExecution(
		ctx context.Context,
		ns domain.Namespace,
		method string,
		vars map[string]string,
	) error
	BeginUpdate(
		ctx context.Context,
		ns domain.Namespace,
		vars map[string]string,
		targetRef string,
	) error
	Forget(
		ctx context.Context,
		ns domain.Namespace,
	) error
}

type Graph interface {
	DiffDeps(
		old, new *domain.Arrow,
	) models.DepDiff
}

// Settler is what a bracket needs from the settling of the updates it began.
type Settler interface {
	Settling(ns domain.Namespace) bool
	RestoreAbandoned(
		ctx context.Context,
		ns domain.Namespace,
	)
}

// Deps is what a bracket needs from the dependency side of an update.
type Deps interface {
	StopIfRunning(
		ctx context.Context,
		ns domain.Namespace,
	) error
	SyncTargetDeps(
		ctx context.Context,
		ns domain.Namespace,
		current *domain.Arrow,
		target *domain.Arrow,
	) error
}
