package usecases

import (
	"context"
	"fmt"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/models/mappers"
	arrowrepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/graph"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/lifecycle"
	runtimerepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
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

	// AdoptInstalled registers ns's identity, named as for Add, as already
	// installed at resolvedRef, a ref its selector could resolve to. The
	// runtime is left alone.
	AdoptInstalled(
		ctx context.Context,
		ns domain.Namespace,
		resolvedRef string,
	) error

	// Update re-resolves what is ahead of ns. A row that is not installed is
	// advanced to it in the catalog; an installed one is left where it is,
	// since only POST /runtime/:ns/update runs the update steps that move it.
	Update(
		ctx context.Context,
		ns domain.Namespace,
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

	GetReadme(
		ctx context.Context,
		ns domain.Namespace,
	) (string, error)

	HasDependents(
		ctx context.Context,
		ns domain.Namespace,
		excludeNs domain.Namespace,
	) (bool, error)

	GetDependents(
		ctx context.Context,
		ns domain.Namespace,
	) ([]domain.Namespace, error)

	GetDependencies(
		ctx context.Context,
		ns domain.Namespace,
	) (models.Plan, error)

	Seed(
		ctx context.Context,
		ns domain.Namespace,
		data []byte,
	) error

	ValidateManifest(
		ctx context.Context,
		data []byte,
	) (*models.ValidationResult, error)

	ListChannels(
		ctx context.Context,
		ns domain.Namespace,
	) ([]models.ChannelInfo, error)

	// Open starts the installed arrow's desktop app, detached from the
	// daemon. It fails with ErrNotOpenable when the arrow is not installed or
	// exposed no desktop entry.
	Open(
		ctx context.Context,
		ns domain.Namespace,
	) error
}

type arrowUsecase struct {
	arrow     arrowrepo.Arrow
	graph     graph.Graph
	runtime   runtimerepo.Runtime
	lifecycle lifecycle.Lifecycle
	launcher  Launcher
}

// NewArrowUsecase wires arrow, graph, and runtime repositories and the
// lifecycle into an ArrowUsecase.
func NewArrowUsecase(
	arrow arrowrepo.Arrow,
	graph graph.Graph,
	runtime runtimerepo.Runtime,
	lc lifecycle.Lifecycle,
	launcher Launcher,
) ArrowUsecase {
	return &arrowUsecase{
		arrow:     arrow,
		graph:     graph,
		runtime:   runtime,
		lifecycle: lc,
		launcher:  launcher,
	}
}

func (u *arrowUsecase) Add(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return u.arrow.Add(ctx, ns)
}

func (u *arrowUsecase) AdoptInstalled(
	ctx context.Context,
	ns domain.Namespace,
	resolvedRef string,
) error {
	return u.arrow.AdoptInstalled(ctx, ns, resolvedRef)
}

func (u *arrowUsecase) Remove(
	ctx context.Context,
	ns domain.Namespace,
) error {
	ns, err := u.arrow.ResolveCatalogued(ctx, ns)
	if err != nil {
		return fmt.Errorf("remove: %w", err)
	}

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
) (models.UpdateResult, error) {
	return u.lifecycle.Recheck(ctx, ns)
}

func (u *arrowUsecase) List(
	ctx context.Context,
	userInstalled *bool,
) ([]models.ArrowListDTO, error) {
	views, err := u.arrow.List(ctx, userInstalled)
	if err != nil {
		return nil, err
	}
	for i, view := range views {
		for j, ver := range view.Versions {
			if state, stateErr := u.runtime.GetState(ctx, ver.Namespace); stateErr == nil {
				views[i].Versions[j].State = state
			}
			views[i].Versions[j].Openable = u.openable(ctx, ver.Namespace, views[i].Versions[j].State)
		}
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
	// view.Metadata.Namespace, not the caller's own ns: a bare namespace (no
	// @ref) is resolved to the tracked ref by u.arrow.GetDetail above, but
	// the runtime repository's aggregates are keyed by the exact ref-
	// qualified namespace string -- passing the caller's still-bare ns
	// through looks up an aggregate that was never written, silently
	// reading back Absent for an arrow that is genuinely installed.
	rt, _ := u.runtime.GetRuntime(ctx, view.Metadata.Namespace)
	if rt != nil {
		view.State = rt.State
		view.ActiveRun = rt.Execution
		view.LastReturn = rt.LastReturn
	}
	view.Openable = u.openable(ctx, view.Metadata.Namespace, view.State)
	return mappers.ArrowDetailDTOFrom(view), nil
}

func (u *arrowUsecase) GetManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*models.ArrowManifestDTO, error) {
	arrow, err := u.arrow.ResolveManifest(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("get manifest: %w", err)
	}
	return mappers.ArrowManifestDTOFrom(arrow), nil
}

func (u *arrowUsecase) GetReadme(
	ctx context.Context,
	ns domain.Namespace,
) (string, error) {
	arrow, err := u.arrow.ResolveManifest(ctx, ns)
	if err != nil {
		return "", fmt.Errorf("get readme: %w", err)
	}
	if arrow.Readme == "" {
		return "", fmt.Errorf("get readme: %w", apperrors.ErrNotFound)
	}
	return arrow.Readme, nil
}

func (u *arrowUsecase) HasDependents(
	ctx context.Context,
	ns domain.Namespace,
	excludeNs domain.Namespace,
) (bool, error) {
	return u.graph.HasDependents(ctx, ns, excludeNs)
}

func (u *arrowUsecase) GetDependents(
	ctx context.Context,
	ns domain.Namespace,
) ([]domain.Namespace, error) {
	return u.graph.GetDependents(ctx, ns)
}

func (u *arrowUsecase) GetDependencies(
	ctx context.Context,
	ns domain.Namespace,
) (models.Plan, error) {
	return u.graph.Resolve(ctx, ns)
}

// seededFilename is what a manifest posted to the seed endpoint is cached as.
const seededFilename = "ARROW.md"

// Seed registers posted manifest bytes as a pin of ns's own ref.
func (u *arrowUsecase) Seed(
	ctx context.Context,
	ns domain.Namespace,
	data []byte,
) error {
	return u.arrow.Adopt(ctx, ns, domain.SelectorPin, domain.Resolved{Ref: ns.Ref()}, data, seededFilename)
}

func (u *arrowUsecase) ValidateManifest(
	ctx context.Context,
	data []byte,
) (*models.ValidationResult, error) {
	return u.arrow.ValidateManifest(ctx, data)
}

func (u *arrowUsecase) ListChannels(
	ctx context.Context,
	ns domain.Namespace,
) ([]models.ChannelInfo, error) {
	return u.arrow.ListChannels(ctx, ns)
}

func (u *arrowUsecase) Open(
	ctx context.Context,
	ns domain.Namespace,
) error {
	ns, err := u.arrow.ResolveCatalogued(ctx, ns)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	state, err := u.runtime.GetState(ctx, ns)
	if err != nil {
		return fmt.Errorf("open: get state: %w", err)
	}
	if state != domain.ArrowStateReady || u.launcher == nil {
		return fmt.Errorf("open %s: %w", ns, apperrors.ErrNotOpenable)
	}
	return u.launcher.Launch(ctx, ns)
}

func (u *arrowUsecase) openable(
	ctx context.Context,
	ns domain.Namespace,
	state domain.ArrowState,
) bool {
	return u.launcher != nil && state == domain.ArrowStateReady && u.launcher.Launchable(ctx, ns)
}
