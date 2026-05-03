package usecases

import (
	"context"
	"fmt"

	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/app/models"
	"github.com/rabbytesoftware/quiver/internal/app/models/mappers"
	arrowrepo "github.com/rabbytesoftware/quiver/internal/app/repositories/arrow"
	"github.com/rabbytesoftware/quiver/internal/app/repositories/graph"
	runtimerepo "github.com/rabbytesoftware/quiver/internal/app/repositories/runtime"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

// ArrowUsecase is the public contract for arrow read/write operations.
type ArrowUsecase interface {
	Add(
		ctx context.Context,
		ns domain.Namespace,
	) error

	Remove(
		ctx context.Context,
		ns domain.Namespace,
	) error

	Update(
		ctx context.Context,
		ns domain.Namespace,
		opts models.UpdateOptions,
	) (models.UpdateResult, error)

	List(
		ctx context.Context,
		userInstalled *bool,
	) ([]models.ArrowListDTO, error)

	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)

	GetDetail(
		ctx context.Context,
		ns domain.Namespace,
	) (*models.ArrowDetailDTO, error)

	GetManifest(
		ctx context.Context,
		ns domain.Namespace,
	) (*models.ArrowManifestDTO, error)

	HasDependents(
		ctx context.Context,
		ns domain.Namespace,
		excludeNs domain.Namespace,
	) (bool, error)

	Seed(
		ctx context.Context,
		ns domain.Namespace,
		data []byte,
	) error

	ValidateManifest(
		ctx context.Context,
		data []byte,
	) (*models.ValidationResult, error)
}

type arrowUsecase struct {
	arrow   arrowrepo.Arrow
	graph   graph.Graph
	runtime runtimerepo.Runtime
}

// NewArrowUsecase wires arrow, graph, and runtime repositories into an ArrowUsecase.
func NewArrowUsecase(
	arrow arrowrepo.Arrow,
	graph graph.Graph,
	runtime runtimerepo.Runtime,
) ArrowUsecase {
	return &arrowUsecase{
		arrow:   arrow,
		graph:   graph,
		runtime: runtime,
	}
}

func (u *arrowUsecase) Add(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return u.arrow.Add(ctx, ns)
}

func (u *arrowUsecase) Remove(
	ctx context.Context,
	ns domain.Namespace,
) error {
	state, err := u.runtime.GetState(ctx, ns)
	if err != nil {
		return fmt.Errorf("remove: get state: %w", err)
	}

	if state.IsActive() {
		return fmt.Errorf("remove: %w", apperrors.ErrStateViolation)
	}

	hasDeps, err := u.graph.HasDependents(ctx, ns, "")
	if err != nil {
		return fmt.Errorf("remove: check dependents: %w", err)
	}
	if hasDeps {
		return fmt.Errorf("remove: %w", apperrors.ErrDependentsExist)
	}

	return u.arrow.Remove(ctx, ns)
}

func (u *arrowUsecase) Update(
	ctx context.Context,
	ns domain.Namespace,
	opts models.UpdateOptions,
) (models.UpdateResult, error) {
	current, err := u.arrow.Get(ctx, ns)
	if err != nil {
		return models.UpdateResult{}, fmt.Errorf("update: get current: %w", err)
	}

	if opts.UpgradeRef && current.InstalledConstraint != "" {
		return u.upgradeRef(ctx, ns, current)
	}

	newArrow, err := u.arrow.ResolveManifest(ctx, ns)
	if err != nil {
		return models.UpdateResult{}, fmt.Errorf("update: fetch manifest: %w", err)
	}

	diff := u.graph.DiffDeps(current, newArrow)

	if err := u.arrow.UpdateManifest(ctx, ns, newArrow); err != nil {
		return models.UpdateResult{}, fmt.Errorf("update: apply manifest: %w", err)
	}

	hasDrift := len(diff.Added) > 0 || len(diff.Removed) > 0
	if hasDrift {
		state, stateErr := u.runtime.GetState(ctx, ns)
		if stateErr == nil && state == domain.ArrowStateReady {
			addedNs := edgesToNs(diff.Added)
			removedNs := edgesToNs(diff.Removed)
			_ = u.runtime.MarkOutdated(ctx, ns, addedNs, removedNs)
		}
	}

	return models.UpdateResult{
		AddedDeps:           edgesToNs(diff.Added),
		RemovedFromManifest: edgesToNs(diff.Removed),
		ConstrainedDeps:     diff.Constrained,
	}, nil
}

func (u *arrowUsecase) upgradeRef(
	ctx context.Context,
	ns domain.Namespace,
	current *domain.Arrow,
) (models.UpdateResult, error) {
	constraint := current.InstalledConstraint

	latestRef, err := u.arrow.ResolveConstraint(ctx, ns, constraint)
	if err != nil {
		return models.UpdateResult{}, fmt.Errorf("upgrade ref: resolve constraint: %w", err)
	}

	newNs := ns.WithRef(latestRef)
	if newNs.String() == ns.String() {
		newArrow, resolveErr := u.arrow.ResolveManifest(ctx, ns)
		if resolveErr != nil {
			return models.UpdateResult{}, fmt.Errorf("upgrade ref: fetch manifest: %w", resolveErr)
		}
		diff := u.graph.DiffDeps(current, newArrow)
		if err := u.arrow.UpdateManifest(ctx, ns, newArrow); err != nil {
			return models.UpdateResult{}, fmt.Errorf("upgrade ref: apply manifest: %w", err)
		}
		return models.UpdateResult{
			AddedDeps:           edgesToNs(diff.Added),
			RemovedFromManifest: edgesToNs(diff.Removed),
			ConstrainedDeps:     diff.Constrained,
		}, nil
	}

	runtimeExists, _ := u.runtime.RuntimeExists(ctx, newNs)

	newArrow, err := u.arrow.UpgradeVersion(ctx, ns, newNs, constraint, runtimeExists)
	if err != nil {
		return models.UpdateResult{}, fmt.Errorf("upgrade ref: upgrade version: %w", err)
	}

	diff := u.graph.DiffDeps(current, newArrow)
	return models.UpdateResult{
		AddedDeps:           edgesToNs(diff.Added),
		RemovedFromManifest: edgesToNs(diff.Removed),
		ConstrainedDeps:     diff.Constrained,
	}, nil
}

func (u *arrowUsecase) List(
	ctx context.Context,
	userInstalled *bool,
) ([]models.ArrowListDTO, error) {
	views, err := u.arrow.List(ctx, userInstalled)
	if err != nil {
		return nil, err
	}
	return mappers.ArrowListDTOsFrom(views), nil
}

func (u *arrowUsecase) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	return u.arrow.Get(ctx, ns)
}

func (u *arrowUsecase) GetDetail(
	ctx context.Context,
	ns domain.Namespace,
) (*models.ArrowDetailDTO, error) {
	view, err := u.arrow.GetDetail(ctx, ns)
	if err != nil {
		return nil, err
	}
	rt, _ := u.runtime.GetRuntime(ctx, ns)
	if rt != nil {
		view.State = rt.State
		view.ActiveRun = rt.Execution
		view.LastReturn = rt.LastReturn
	}
	return mappers.ArrowDetailDTOFrom(view), nil
}

func (u *arrowUsecase) GetManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*models.ArrowManifestDTO, error) {
	if ns.Ref() != "" {
		return nil, fmt.Errorf("get manifest: %w", apperrors.ErrInvalidNamespace)
	}

	arrow, err := u.arrow.ResolveManifest(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("get manifest: %w", err)
	}
	return mappers.ArrowManifestDTOFrom(arrow), nil
}

func (u *arrowUsecase) HasDependents(
	ctx context.Context,
	ns domain.Namespace,
	excludeNs domain.Namespace,
) (bool, error) {
	return u.graph.HasDependents(ctx, ns, excludeNs)
}

func (u *arrowUsecase) Seed(
	ctx context.Context,
	ns domain.Namespace,
	data []byte,
) error {
	return u.arrow.Seed(ctx, ns, data)
}

func edgesToNs(edges []domain.DependencyEdge) []domain.Namespace {
	ns := make([]domain.Namespace, 0, len(edges))
	for _, e := range edges {
		ns = append(ns, e.Namespace)
	}
	return ns
}

func (u *arrowUsecase) ValidateManifest(
	ctx context.Context,
	data []byte,
) (*models.ValidationResult, error) {
	return u.arrow.ValidateManifest(ctx, data)
}
