package usecases

import (
	"context"
	"fmt"
	"slices"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/models/mappers"
	arrowrepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/graph"
	runtimerepo "github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// ArrowUsecase is the public contract for arrow read/write operations.
type ArrowUsecase interface {
	Add(
		ctx context.Context,
		ns domain.Namespace,
		opts models.AddOptions,
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
	opts models.AddOptions,
) error {
	return u.arrow.Add(ctx, ns, opts)
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
	opts models.UpdateOptions,
) (models.UpdateResult, error) {
	ns, err := u.arrow.ResolveCatalogued(ctx, ns)
	if err != nil {
		return models.UpdateResult{}, fmt.Errorf("update: %w", err)
	}

	current, err := u.arrow.Get(ctx, ns)
	if err != nil {
		return models.UpdateResult{}, fmt.Errorf("update: get current: %w", err)
	}

	if opts.Channel != "" {
		return u.switchChannel(ctx, ns, current, opts)
	}

	if opts.UpgradeRef && current.InstalledConstraint != "" {
		// A ref swap is safe on a running arrow once it is stopped first: the
		// same requirement upgrading any running service has, not specific to
		// any one arrow. Without this, UpgradeVersion's own reaction
		// (onArrowUpgraded) would delete the running row and never pick up the
		// new one, since it only auto-continues from Ready/Outdated.
		if err := u.stopIfRunning(ctx, ns); err != nil {
			return models.UpdateResult{}, fmt.Errorf("update: stop before upgrade: %w", err)
		}
		return u.upgradeRef(ctx, ns, current)
	}

	return u.plainRefresh(ctx, ns, current)
}

// plainRefresh is Update's default path: a plain manifest refresh with
// neither a channel switch nor an upgrade_ref requested, guarded against a
// running arrow. Split out of Update to keep that function under the
// cyclomatic complexity limit.
func (u *arrowUsecase) plainRefresh(
	ctx context.Context,
	ns domain.Namespace,
	current *domain.Arrow,
) (models.UpdateResult, error) {
	if state, stateErr := u.runtime.GetState(ctx, ns); stateErr == nil && state == domain.ArrowStateRunning {
		return models.UpdateResult{}, fmt.Errorf("update: %w", apperrors.ErrStateViolation)
	}

	newArrow, err := u.arrow.RefreshManifest(ctx, ns)
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
		if stateErr == nil && (state == domain.ArrowStateReady || state == domain.ArrowStateOutdated) {
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
	if newNs.String() == ns.String() { //nolint:nestif
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

	newArrow, err := u.arrow.UpgradeVersion(ctx, ns, newNs, constraint, runtimeExists, false)
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

// switchChannel moves ns onto a different release channel, optionally
// pinning to a specific ref within it instead of taking the channel's
// latest. The channel is recorded even when the target ref turns out to be
// the one ns is already at — only the (safe to skip) ref swap is skipped in
// that case.
func (u *arrowUsecase) switchChannel(
	ctx context.Context,
	ns domain.Namespace,
	current *domain.Arrow,
	opts models.UpdateOptions,
) (models.UpdateResult, error) {
	channels, err := u.arrow.ListChannels(ctx, ns)
	if err != nil {
		return models.UpdateResult{}, fmt.Errorf("switch channel: list channels: %w", err)
	}

	entry, found := findChannel(channels, opts.Channel)
	if !found {
		return models.UpdateResult{}, fmt.Errorf(
			"switch channel: channel %s: %w", opts.Channel, apperrors.ErrChannelNotFound,
		)
	}

	targetRef := entry.Latest
	if opts.Ref != "" && !channelHasRef(entry, opts.Ref) {
		return models.UpdateResult{}, fmt.Errorf(
			"switch channel: ref %s not in channel %s: %w", opts.Ref, opts.Channel, apperrors.ErrChannelNotFound,
		)
	}
	if opts.Ref != "" {
		targetRef = opts.Ref
	}

	if err := u.arrow.SetChannel(ctx, ns, opts.Channel); err != nil {
		return models.UpdateResult{}, fmt.Errorf("switch channel: set channel: %w", err)
	}

	newNs := ns.WithRef(targetRef)
	if newNs.String() == ns.String() {
		return models.UpdateResult{}, nil
	}

	// Same running-arrow safety requirement upgradeRef already has: a ref
	// swap needs the arrow stopped first.
	if err := u.stopIfRunning(ctx, ns); err != nil {
		return models.UpdateResult{}, fmt.Errorf("switch channel: stop before upgrade: %w", err)
	}

	runtimeExists, _ := u.runtime.RuntimeExists(ctx, newNs)

	newArrow, err := u.arrow.UpgradeVersion(ctx, ns, newNs, current.InstalledConstraint, runtimeExists, false)
	if err != nil {
		return models.UpdateResult{}, fmt.Errorf("switch channel: upgrade version: %w", err)
	}

	// UpgradeVersion's own row (arrow.upgraded) carries no Channel of its
	// own, unlike a plain Add — it swaps the catalog identity onto newNs
	// from a freshly resolved manifest, which has no channel opinion. The
	// earlier SetChannel call landed on ns, which this same upgrade just
	// forgot, so the tracked channel is re-stamped here, onto the row that
	// actually survives.
	if err := u.arrow.SetChannel(ctx, newNs, opts.Channel); err != nil {
		return models.UpdateResult{}, fmt.Errorf("switch channel: set channel on new ref: %w", err)
	}

	diff := u.graph.DiffDeps(current, newArrow)
	return models.UpdateResult{
		AddedDeps:           edgesToNs(diff.Added),
		RemovedFromManifest: edgesToNs(diff.Removed),
		ConstrainedDeps:     diff.Constrained,
	}, nil
}

// findChannel returns the channel entry named name, if channels has one.
func findChannel(channels []models.ChannelInfo, name string) (models.ChannelInfo, bool) {
	for _, c := range channels {
		if c.Name == name {
			return c, true
		}
	}
	return models.ChannelInfo{}, false
}

// channelHasRef reports whether ref is a legitimate member of entry: any
// listed member for an ordered channel, or only entry's own Latest for a
// pointer channel, which by definition has no other members.
func channelHasRef(entry models.ChannelInfo, ref string) bool {
	if entry.Kind == "pointer" {
		return ref == entry.Latest
	}
	return slices.Contains(entry.Members, ref)
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

func (u *arrowUsecase) Seed(
	ctx context.Context,
	ns domain.Namespace,
	data []byte,
) error {
	return u.arrow.Seed(ctx, ns, data)
}

// stopIfRunning stops ns and waits for the stop to finish before returning,
// so a caller that swaps ns's row right afterward never races a still-running
// execution. A no-op for any state other than Running.
func (u *arrowUsecase) stopIfRunning(
	ctx context.Context,
	ns domain.Namespace,
) error {
	state, err := u.runtime.GetState(ctx, ns)
	if err != nil {
		return fmt.Errorf("get state: %w", err)
	}
	if state != domain.ArrowStateRunning {
		return nil
	}

	ch, unsub, err := u.runtime.ListenEnded(ctx, ns)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer unsub()

	if err := u.runtime.BeginStop(ctx, ns); err != nil {
		return fmt.Errorf("begin stop: %w", err)
	}

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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

func (u *arrowUsecase) ListChannels(
	ctx context.Context,
	ns domain.Namespace,
) ([]models.ChannelInfo, error) {
	return u.arrow.ListChannels(ctx, ns)
}
