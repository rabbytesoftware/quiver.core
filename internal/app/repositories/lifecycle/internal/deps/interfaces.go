package deps

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
	Exists(
		ctx context.Context,
		ns domain.Namespace,
	) (bool, error)
	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
	AddDependency(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.Namespace, error)
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
	BeginInstall(
		ctx context.Context,
		ns domain.Namespace,
		vars map[string]string,
	) error
	BeginExecution(
		ctx context.Context,
		ns domain.Namespace,
		method string,
		vars map[string]string,
	) error
	BeginStop(
		ctx context.Context,
		ns domain.Namespace,
	) error
	BeginUninstall(
		ctx context.Context,
		ns domain.Namespace,
		vars map[string]string,
	) error
	ListenEnded(
		ctx context.Context,
		ns domain.Namespace,
	) (<-chan domainRuntime.ArrowRuntime, func(), error)
	MarkOutdated(
		ctx context.Context,
		ns domain.Namespace,
		addedDeps []domain.Namespace,
		removedDeps []domain.Namespace,
	) error
}

type Graph interface {
	Resolve(
		ctx context.Context,
		ns domain.Namespace,
	) (models.Plan, error)
	HasDependents(
		ctx context.Context,
		ns domain.Namespace,
		excludeNs domain.Namespace,
	) (bool, error)
	GetDependents(
		ctx context.Context,
		ns domain.Namespace,
	) ([]domain.Namespace, error)
	DiffDeps(
		old, new *domain.Arrow,
	) models.DepDiff
}

// Brackets serializes what moves or begins one row.
type Brackets interface {
	Open(
		ctx context.Context,
		ns domain.Namespace,
	) (func(), error)
}
