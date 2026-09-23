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
		return u.switchChannel(ctx, ns, opts)
	}

	// A constraint-tracked arrow upgrades toward its constraint's latest
	// match; a channel-tracked one (no constraint at all, the normal shape
	// once an arrow's channel has been set) upgrades toward that channel's
	// latest instead — upgradeRef resolves either the same way
	// checkTagDrift's own drift check does, via ResolveTrackedRef.
	if opts.UpgradeRef && (current.InstalledConstraint != "" || current.Channel != "") {
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

// upgradeRef resolves ns's next ref and moves the catalog row onto it.
//
// RecommendedRef is preferred outright when the arrow already carries one:
// it is the exact value the passive drift-check (checkTagDrift) already
// computed and the API is already showing as "the recommended upgrade", so
// reusing it guarantees the badge and the actual upgrade can never disagree,
// and it costs no live resolution the check has not already paid for. A
// fresh ResolveTrackedRef call — the identical constraint-first,
// channel-fallback rule checkTagDrift itself uses — covers the remaining
// case: an arrow never yet checked, or genuinely already at the latest.
func (u *arrowUsecase) upgradeRef(
	ctx context.Context,
	ns domain.Namespace,
	current *domain.Arrow,
) (models.UpdateResult, error) {
	latestRef := current.RecommendedRef
	if latestRef == "" {
		resolved, err := u.arrow.ResolveTrackedRef(ctx, *current)
		if err != nil {
			return models.UpdateResult{}, fmt.Errorf("upgrade ref: resolve tracked ref: %w", err)
		}
		latestRef = resolved
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

	// current.Channel travels with the upgrade in the SAME event
	// (UpgradeArrow.EmitEvent sets it directly) rather than via a follow-up
	// SetChannel call: SetChannel's own EmitEvent clears InstalledConstraint
	// unconditionally, which is correct for an explicit channel switch but
	// wrong here -- nothing about the channel changed on an ordinary
	// upgrade, only the ref moved within the same tracking, so the
	// constraint (if any) must survive untouched.
	newArrow, err := u.arrow.UpgradeVersion(ctx, ns, newNs, current.InstalledConstraint, current.Channel, runtimeExists, false)
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

// switchChannel records which release channel ns should track from now on.
// It never performs a live upgrade inline: that is exclusively upgradeRef's
// job (via Update's UpgradeRef flag), reached either by an explicit click or
// by whatever surfaces the outdated badge this triggers. Validation still
// checks a specific pinned ref (opts.Ref) belongs to the channel, matching
// today's error behaviour exactly, but a valid pin is not otherwise carried
// forward: nothing downstream (ResolveTrackedRef, the passive drift-check)
// understands "channel X, pinned to ref Y" as a resolution target, only
// "channel X's latest" — pinning to an exact ref within a channel is not
// this round's scope.
func (u *arrowUsecase) switchChannel(
	ctx context.Context,
	ns domain.Namespace,
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
	if opts.Ref != "" && !channelHasRef(entry, opts.Ref) {
		return models.UpdateResult{}, fmt.Errorf(
			"switch channel: ref %s not in channel %s: %w", opts.Ref, opts.Channel, apperrors.ErrChannelNotFound,
		)
	}

	if err := u.arrow.SetChannel(ctx, ns, opts.Channel); err != nil {
		return models.UpdateResult{}, fmt.Errorf("switch channel: set channel: %w", err)
	}

	// Fire-and-forget: the caller gets its response the instant the
	// preference is durably recorded, not after a live git resolve. If the
	// new channel is ahead, this lands Outdated/RecommendedRef moments
	// later — the same outdated-detection + explicit-Update flow every
	// other arrow already goes through, not a channel-specific special case.
	u.arrow.CheckVersionNow(ctx, ns)

	return models.UpdateResult{}, nil
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

// channelHasRef reports whether ref is a legitimate pin within entry: any
// listed member for an ordered channel, restricted to its fixed release
// set, or any non-empty ref at all for a pointer channel — a rolling
// channel (a branch, or an unversioned tag) has no fixed member list by
// definition, so a caller may pin to any ref they choose under it (a
// specific commit, a differently named tag, whatever). A ref that turns
// out not to exist fails later at UpgradeVersion's manifest fetch, the same
// way a plain Add with an arbitrary explicit ref already behaves today.
func channelHasRef(entry models.ChannelInfo, ref string) bool {
	if entry.Kind == "pointer" {
		return ref != ""
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
