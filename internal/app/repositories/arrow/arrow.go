package arrow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	apphub "github.com/rabbytesoftware/quiver.core/internal/app/hub"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	arrowcmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/commands"
	arrowstore "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

type Arrow interface {
	List(
		ctx context.Context,
		userInstalled *bool,
	) ([]models.ArrowView, error)
	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
	Exists(
		ctx context.Context,
		ns domain.Namespace,
	) (bool, error)
	GetDetail(
		ctx context.Context,
		ns domain.Namespace,
	) (*models.ArrowDetailView, error)
	GetManifest(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
	ResolveManifest(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
	RefreshManifest(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
	ResolveForInstall(
		ctx context.Context,
		ns domain.Namespace,
		channel string,
	) (
		resolvedNs domain.Namespace,
		arrow *domain.Arrow,
		constraint string,
		err error,
	)
	// ResolveCatalogued maps a namespace as the caller typed it onto the one
	// the catalog holds it under, so a refless namespace reaches the runtime
	// verbs as the ref they were catalogued with.
	ResolveCatalogued(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.Namespace, error)
	Search(
		ctx context.Context,
		q models.SearchQuery,
	) ([]models.CatalogHit, error)

	Add(
		ctx context.Context,
		ns domain.Namespace,
		opts models.AddOptions,
	) error
	AddDep(
		ctx context.Context,
		ns domain.Namespace,
		arrow *domain.Arrow,
		constraint string,
	) error
	Remove(
		ctx context.Context,
		ns domain.Namespace,
	) error
	Seed(
		ctx context.Context,
		ns domain.Namespace,
		data []byte,
	) error
	ValidateManifest(
		ctx context.Context,
		data []byte,
	) (*models.ValidationResult, error)
	// MarkInstalled records when the arrow's ref was installed.
	MarkInstalled(
		ctx context.Context,
		ns domain.Namespace,
		at time.Time,
	) error
	// MarkUninstalled clears the stamp MarkInstalled recorded.
	MarkUninstalled(
		ctx context.Context,
		ns domain.Namespace,
	) error
	// MarkLastUsed records when the arrow's ref last completed an _execute run.
	MarkLastUsed(
		ctx context.Context,
		ns domain.Namespace,
		at time.Time,
	) error
	// SetChannel changes which release channel ns tracks. ref, when
	// non-empty, pins ns to that exact ref within channel rather than the
	// channel's own latest; empty clears any previously pinned ref.
	SetChannel(
		ctx context.Context,
		ns domain.Namespace,
		channel string,
		ref string,
	) error
	// CheckVersionNow launches an immediate version-drift check for ns,
	// bypassing the TTL GetDetail's passive maybeCheckVersion otherwise
	// enforces. For a deliberate user action that just changed what ns
	// tracks (switchChannel's SetChannel), so the outdated badge does not
	// wait up to version_check_ttl to notice. Launched detached, the same
	// way maybeCheckVersion already launches its own check — it does not
	// block the caller.
	CheckVersionNow(
		ctx context.Context,
		ns domain.Namespace,
	)
	Forget(
		ctx context.Context,
		ns domain.Namespace,
	) error
	UpdateManifest(
		ctx context.Context,
		ns domain.Namespace,
		arrow *domain.Arrow,
	) error
	ResolveConstraint(
		ctx context.Context,
		ns domain.Namespace,
		constraint string,
	) (ref string, err error)
	// ResolveLatestStable resolves ns to the ref of its latest stable release,
	// the same fallback checkTagDrift already uses for an arrow installed at
	// an exact ref with no tracked constraint.
	ResolveLatestStable(
		ctx context.Context,
		ns domain.Namespace,
	) (ref string, err error)
	// ResolveTrackedRef resolves arrow's next ref the same way the passive
	// version-drift check does: constraint-first, tracked-channel fallback
	// otherwise. See arrowstore.Store.ResolveTrackedRef.
	ResolveTrackedRef(
		ctx context.Context,
		arrow domain.Arrow,
	) (ref string, err error)
	// ListChannels reports every channel ns's repository publishes.
	ListChannels(
		ctx context.Context,
		ns domain.Namespace,
	) ([]models.ChannelInfo, error)
	UpgradeVersion(
		ctx context.Context,
		oldNs domain.Namespace,
		newNs domain.Namespace,
		constraint string,
		channel string,
		runtimeAlreadyExists bool,
		alreadyReady bool,
		userInstalled bool,
		pinnedRef string,
	) (*domain.Arrow, error)
	// UpgradeVersionSeeded is UpgradeVersion's network-free counterpart: the
	// caller already holds newNs's manifest bytes (the same shape Seed
	// accepts) and needs no remote fetch to move the row's identity onto it.
	// It always lands the new row straight at Ready.
	UpgradeVersionSeeded(
		ctx context.Context,
		oldNs domain.Namespace,
		newNs domain.Namespace,
		data []byte,
	) error
	Shutdown(
		ctx context.Context,
	) error

	// OnArrowUpdated carries the full *domain.Arrow so graph.SyncDependencies can be called directly without re-fetching.
	OnArrowAdded(fn func(
		ctx context.Context,
		ns domain.Namespace,
		arrow domain.Arrow,
	) error) error
	OnArrowUpdated(fn func(
		ctx context.Context,
		ns domain.Namespace,
		arrow *domain.Arrow,
	) error) error
	OnArrowRemoved(fn func(
		ctx context.Context,
		ns domain.Namespace,
	) error) error
	// OnArrowUpgraded fires on arrow.upgraded.* events, carrying the new Arrow
	// with UpgradedFromNs set so reactions can coordinate old → new cleanup.
	OnArrowUpgraded(fn func(
		ctx context.Context,
		arrow domain.Arrow,
	) error) error
}

type arrowService struct {
	store    arrowstore.Store
	axArrow  asynx.Asynx[domain.Arrow]
	vault    vault.Vault
	manifold manifold.Manifold
	hub      apphub.WebSocketHub

	// preinstalled is zero unless WithPreinstalledDetection was passed, which
	// is what makes Add's behaviour for every existing arrow unchanged.
	preinstalled preinstalledOpts

	// versionOutdatedSync is zero unless WithVersionOutdatedSync was passed,
	// which is what keeps a version check's effect confined to the Arrow
	// aggregate for a catalog built without a runtime to talk to.
	versionOutdatedSync SetVersionOutdatedFn

	// asynx runs one goroutine per subscriber, so a second subscription on an
	// arrow topic would race the read-model write and the reactions alike.
	// Callbacks are held here and invoked by the single projection instead, in
	// the order the invariant needs.
	callbacksMu sync.RWMutex
	addedFns    []func(ctx context.Context, ns domain.Namespace, arrow domain.Arrow) error
	updatedFns  []func(ctx context.Context, ns domain.Namespace, arrow *domain.Arrow) error
	upgradedFns []func(ctx context.Context, arrow domain.Arrow) error
	removedFns  []func(ctx context.Context, ns domain.Namespace) error
}

func New(
	db *gormdb.DB,
	axArrow asynx.Asynx[domain.Arrow],
	v vault.Vault,
	m manifold.Manifold,
	hub apphub.WebSocketHub,
	opts ...Option,
) (Arrow, error) {
	r, err := arrowstore.New(db, v, m)
	if err != nil {
		return nil, fmt.Errorf("catalog: store: %w", err)
	}

	o := resolveOptions(opts)
	s := &arrowService{
		store:               r,
		axArrow:             axArrow,
		vault:               v,
		manifold:            m,
		hub:                 hub,
		preinstalled:        o.preinstalled,
		versionOutdatedSync: o.versionOutdatedSync,
	}

	if err := s.registerProjections(); err != nil {
		return nil, err
	}

	return s, nil
}

// registerProjections claims one subscriber per arrow topic. Everything an
// arrow event has to do — reactions, read model, broadcast — happens inside
// that subscriber, because asynx gives concurrent subscribers no order and the
// order is the whole point.
func (s *arrowService) registerProjections() error {
	topics := []struct {
		topic   string
		project asynxModels.ProjectionHandler[domain.Arrow]
	}{
		{"arrow.added.*", s.projectAdded},
		{"arrow.upgraded.*", s.projectUpgraded},
		{"arrow.updated.*", s.projectUpdated},
		{"arrow.installed.*", s.projectInstallStamp},
		{"arrow.uninstalled.*", s.projectInstallStamp},
		{"arrow.version_checked.*", s.projectVersionCheck},
		{"arrow.channel_set.*", s.projectChannelSet},
	}

	for _, t := range topics {
		if _, err := s.axArrow.Subscribe(asynx.Topic(t.topic), t.project); err != nil {
			return fmt.Errorf("catalog projection: subscribe %s: %w", t.topic, err)
		}
	}

	if _, err := s.axArrow.OnForget(s.projectForgotten); err != nil {
		return fmt.Errorf("catalog projection: subscribe arrow forget: %w", err)
	}

	return nil
}

// project runs the reactions the arrow's usability depends on, then makes the
// arrow readable, then announces it.
//
// Dependency edges come first because they are what decides whether an arrow
// may be removed: an arrow readable in the catalog with no edges yet lets
// something it depends on be deleted out from under it. The broadcast comes
// last because a client told an arrow exists will go and read it.
//
// A read model that could not be written is never announced — announcing state
// nobody can read is the failure this ordering exists to prevent.
//
// A reaction that fails is logged and the arrow is still written. Nothing
// rebuilds the catalog from the event stream, so refusing the write would hide
// the arrow permanently, with no way left to even remove it; the next event for
// the same arrow re-runs the reaction.
func (s *arrowService) project(
	ctx context.Context,
	arrow domain.Arrow,
	react func(ctx context.Context, arrow domain.Arrow),
) {
	if react != nil {
		react(ctx, arrow)
	}

	if err := s.store.Project(ctx, arrow); err != nil {
		slog.ErrorContext(ctx, "catalog projection: write read model",
			"ns", arrow.Namespace, "err", err)
		return
	}

	s.broadcast(apphub.ArrowEvent{Kind: apphub.CatalogUpserted, Arrow: arrow})
}

func (s *arrowService) projectAdded(
	ctx context.Context,
	evt asynxModels.Event[domain.Arrow],
) {
	s.project(ctx, evt.Aggregate, s.runAdded)
}

func (s *arrowService) projectUpdated(
	ctx context.Context,
	evt asynxModels.Event[domain.Arrow],
) {
	s.project(ctx, evt.Aggregate, s.runUpdated)
}

func (s *arrowService) projectUpgraded(
	ctx context.Context,
	evt asynxModels.Event[domain.Arrow],
) {
	s.project(ctx, evt.Aggregate, s.runUpgraded)
}

// projectInstallStamp carries the installed-ref stamp into the read model — set
// by an install, cleared by an uninstall. Nothing derived hangs off it, so there
// is no reaction to run first.
func (s *arrowService) projectInstallStamp(
	ctx context.Context,
	evt asynxModels.Event[domain.Arrow],
) {
	s.project(ctx, evt.Aggregate, nil)
}

// projectVersionCheck carries the outdated/recommended-ref stamp into the
// read model. Nothing derived hangs off it, so there is no reaction to run
// first — same shape as projectInstallStamp.
func (s *arrowService) projectVersionCheck(
	ctx context.Context,
	evt asynxModels.Event[domain.Arrow],
) {
	s.project(ctx, evt.Aggregate, nil)
}

// projectChannelSet carries the tracked-channel stamp into the read model.
// Nothing derived hangs off it, so there is no reaction to run first — same
// shape as projectInstallStamp/projectVersionCheck. Needed because
// switchChannel (usecases/arrow.go) now only ever sends SetChannel against
// an already-installed row, with no upgrade of its own to ride along on:
// without this, the channel would land on the aggregate but never reach the
// read model at all whenever the picked channel's latest already equals the
// installed ref (the one case an ensuing version-check finds no drift to
// report either).
func (s *arrowService) projectChannelSet(
	ctx context.Context,
	evt asynxModels.Event[domain.Arrow],
) {
	s.project(ctx, evt.Aggregate, nil)
}

// projectForgotten mirrors project. The read-model row goes first: an arrow is
// readable only while its dependency edges exist, so the edges may only be
// dropped once nothing can read the arrow any more. The removal is announced
// last, once every trace of the arrow is gone.
func (s *arrowService) projectForgotten(
	ctx context.Context,
	evt asynxModels.Event[domain.Arrow],
) {
	arrow := evt.Aggregate

	if err := s.store.ProjectForget(ctx, arrow); err != nil {
		slog.ErrorContext(ctx, "catalog projection: delete from read model",
			"ns", arrow.Namespace, "err", err)
		return
	}

	s.runRemoved(ctx, arrow.Namespace)
	s.deleteWorkDir(ctx, arrow.Namespace)

	s.broadcast(apphub.ArrowEvent{Kind: apphub.CatalogRemoved, Arrow: arrow})
}

func (s *arrowService) broadcast(
	evt apphub.ArrowEvent,
) {
	if s.hub == nil {
		return
	}
	s.hub.BroadcastArrow(evt)
}

func (s *arrowService) deleteWorkDir(
	ctx context.Context,
	ns domain.Namespace,
) {
	if s.vault == nil {
		return
	}
	if err := s.vault.DeleteWorkDir(ctx, ns); err != nil {
		slog.WarnContext(ctx, "catalog: vault forget: delete work dir failed",
			"ns", ns, "err", err)
	}
}

func (s *arrowService) List(
	ctx context.Context,
	userInstalled *bool,
) ([]models.ArrowView, error) {
	return s.store.List(ctx, userInstalled)
}

func (s *arrowService) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	return s.store.Get(ctx, ns)
}

func (s *arrowService) Exists(
	ctx context.Context,
	ns domain.Namespace,
) (bool, error) {
	return s.axArrow.Exists(ctx, ns.String())
}

const versionCheckTimeout = 30 * time.Second

func (s *arrowService) GetDetail(
	ctx context.Context,
	ns domain.Namespace,
) (*models.ArrowDetailView, error) {
	view, err := s.store.GetDetail(ctx, ns)
	if err != nil {
		return nil, err
	}
	if view != nil {
		s.maybeCheckVersion(ctx, view.Metadata, view.LastVersionCheckAt)
	}
	return view, nil
}

// maybeCheckVersion decides staleness in memory first, from the timestamp
// GetDetail already fetched in the same read — the common case, a call well
// within the TTL, returns here without touching the database at all. Only
// when that looks stale does it fall through to NeedsVersionCheck's atomic
// claim, so concurrent callers for the same namespace still only launch one
// check. The check itself launches detached from ctx: ctx dies with this
// request, but the check must outlive it. A namespace with no catalog row
// (the live-preview path GetDetail falls back to for an uncatalogued
// namespace) carries a zero LastVersionCheckAt, which always looks stale —
// NeedsVersionCheck's own claim then finds no row and answers false, so this
// costs one extra query there, never a check.
func (s *arrowService) maybeCheckVersion(
	ctx context.Context,
	arrow domain.Arrow,
	lastCheckedAt time.Time,
) {
	needs, err := s.store.NeedsVersionCheck(ctx, arrow.Namespace, lastCheckedAt)
	if err != nil || !needs {
		return
	}

	checkCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), versionCheckTimeout)
	go func() {
		defer cancel()
		s.runVersionCheck(checkCtx, arrow)
	}()
}

// runVersionCheck re-resolves arrow against the remote and lands the outcome on
// both aggregates it concerns. A check that cannot produce a trustworthy answer
// (ok is false) writes nothing at all — a failed resolution is no more a
// trustworthy "no drift" than it is a drift.
//
// The runtime state is reconciled first, and unconditionally. First, because
// the intermediate window then reads as a badge a few milliseconds early rather
// than as the missing badge this whole path exists to fix, and because the
// state is what gates whether the arrow can be run at all. Unconditionally,
// because the catalog record below is only written when the answer changed —
// and every arrow already carrying Outdated from before the runtime was wired
// in at all takes that early return on every later check. A reconcile placed
// after it would never run for exactly the installs that need repairing. Doing
// it on every check instead costs one aggregate read per check and makes any
// divergence, however it arose, heal itself at the next one.
func (s *arrowService) runVersionCheck(
	ctx context.Context,
	arrow domain.Arrow,
) {
	outdated, recommendedRef, ok := s.store.CheckVersionDrift(ctx, arrow)
	if !ok {
		return
	}

	s.syncVersionOutdated(ctx, arrow.Namespace, outdated)

	current, err := s.axArrow.Get(ctx, arrow.Namespace.String())
	if err != nil {
		return
	}
	if current.Outdated == outdated && current.RecommendedRef == recommendedRef {
		return
	}

	if _, sendErr := s.axArrow.SendWait(ctx, arrowcmds.RecordVersionCheck{
		Namespace:      arrow.Namespace,
		Outdated:       outdated,
		RecommendedRef: recommendedRef,
	}); sendErr != nil {
		slog.WarnContext(ctx, "arrow version check: record", "ns", arrow.Namespace, "err", sendErr)
	}
}

// CheckVersionNow launches a version-drift check for ns immediately,
// bypassing NeedsVersionCheck's TTL claim entirely — unlike maybeCheckVersion,
// which only launches one once the TTL says it is due. It exists for a
// deliberate user action that just changed what ns tracks: switchChannel
// calls this right after SetChannel so the outdated badge does not wait up to
// version_check_ttl (an hour, by default) to notice a choice the user just
// made.
//
// The seed value comes from axArrow, not the read model: Send is not
// fire-and-forget in the sense that matters here — it blocks until the
// command's event is durably appended, and only skips waiting for
// subscriber/projection handlers to run (that is what SendWait adds on
// top). So by the time SetChannel's call in switchChannel returns, the
// aggregate itself is guaranteed to reflect it for any later Get. The read
// model, by contrast, only catches up once its own projection subscriber
// runs — for SetChannel that could still be pending, and reading it here
// would risk resolving this check against the channel ns tracked before
// the switch.
//
// Launched detached, the same way maybeCheckVersion already launches its own
// check, so the caller's response returns immediately without waiting for a
// live git resolve to finish. A namespace with no aggregate at all, or any
// other lookup failure, is silently a no-op — fire-and-forget by design.
func (s *arrowService) CheckVersionNow(
	ctx context.Context,
	ns domain.Namespace,
) {
	arrow, err := s.axArrow.Get(ctx, ns.String())
	if err != nil {
		return
	}

	checkCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), versionCheckTimeout)
	go func() {
		defer cancel()
		s.runVersionCheck(checkCtx, arrow)
	}()
}

func (s *arrowService) GetManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	return s.store.GetManifest(ctx, ns)
}

func (s *arrowService) ResolveManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	return s.store.ResolveManifest(ctx, ns)
}

// RefreshManifest purges the cached manifest, then resolves it — forcing a
// re-fetch from source rather than returning a still-fresh cached copy.
func (s *arrowService) RefreshManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	if s.vault != nil {
		if err := s.vault.DeleteArrow(ctx, ns); err != nil {
			slog.WarnContext(ctx, "catalog: refresh: purge manifest cache failed",
				"ns", ns, "err", err)
		}
	}
	return s.store.ResolveManifest(ctx, ns)
}

func (s *arrowService) ResolveForInstall(
	ctx context.Context,
	ns domain.Namespace,
	channel string,
) (resolvedNs domain.Namespace, arrow *domain.Arrow, constraint string, err error) {
	return s.store.ResolveForInstall(ctx, ns, channel)
}

func (s *arrowService) ResolveCatalogued(
	ctx context.Context,
	ns domain.Namespace,
) (domain.Namespace, error) {
	return s.store.ResolveCatalogued(ctx, ns)
}

func (s *arrowService) Search(
	ctx context.Context,
	q models.SearchQuery,
) ([]models.CatalogHit, error) {
	return s.store.Search(ctx, q)
}

// appSentinels are the app-layer classifications an error may already carry.
// mapResolveErr consults them so a precise classification made downstream is
// not overwritten by this one.
var appSentinels = []error{
	apperrors.ErrNotFound,
	apperrors.ErrAlreadyExists,
	apperrors.ErrStateViolation,
	apperrors.ErrMethodNotFound,
	apperrors.ErrFetchFailed,
	apperrors.ErrInvalidNamespace,
	apperrors.ErrDependentsExist,
	apperrors.ErrInvalidManifest,
	apperrors.ErrPlatformNotSupported,
	apperrors.ErrMissingVariable,
	apperrors.ErrReservedVariable,
	apperrors.ErrInvalidConfig,
}

// mapResolveErr classifies a manifest-resolution failure.
//
// manifold attaches no app sentinel to its own errors, so without this every
// rejected manifest and every unreachable remote arrives at the API carrying
// nothing errors.Is can match — and apierr.StatusAndMessage answers every one
// of them with 500 "internal error", discarding the chain that said what was
// actually wrong.
//
// The remaining default is deliberate: reaching a manifest is I/O against a
// remote, so an unclassified failure there is a gateway problem rather than a
// server fault.
func mapResolveErr(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, ruleset.ErrNoSupportedPlatform):
		return fmt.Errorf("%w: %w", apperrors.ErrPlatformNotSupported, err)
	case errors.Is(err, ruleset.ErrInvalidManifest):
		return fmt.Errorf("%w: %w", apperrors.ErrInvalidManifest, err)
	}

	for _, sentinel := range appSentinels {
		if errors.Is(err, sentinel) {
			return err
		}
	}

	return fmt.Errorf("%w: %w", apperrors.ErrFetchFailed, err)
}

// Add resolves ns against its remote and writes it into the catalog as
// user-installed.
//
// An arrow whose resolved manifest declares a preinstalled lifecycle for this
// platform is probed first, and a positive detection lands its runtime at Ready
// before the catalog row is written at all — see markIfPreinstalled for why
// that order is the contract and not an optimisation. Every other arrow, which
// is every arrow that does not opt in, takes exactly the path it always did.
func (s *arrowService) Add(
	ctx context.Context,
	ns domain.Namespace,
	opts models.AddOptions,
) error {
	resolvedNs, arrow, constraint, err := s.store.ResolveForInstall(ctx, ns, opts.Channel)
	if err != nil {
		return fmt.Errorf("add: %w", mapResolveErr(err))
	}
	arrow.UserInstalled = true
	arrow.InstalledConstraint = constraint
	if err := s.markIfPreinstalled(ctx, resolvedNs, arrow); err != nil {
		return err
	}
	return s.addArrowCommand(ctx, resolvedNs, arrow, constraint)
}

func (s *arrowService) AddDep(
	ctx context.Context,
	ns domain.Namespace,
	arrow *domain.Arrow,
	constraint string,
) error {
	return s.addArrowCommand(ctx, ns, arrow, constraint)
}

// addArrowCommand waits for the projections rather than only for the write.
// The caller's next move is to install the arrow, and installing reads the
// dependency edges this event produces; returning before they exist is what
// lets a still-depended-on arrow be removed.
func (s *arrowService) addArrowCommand(
	ctx context.Context,
	ns domain.Namespace,
	arrow *domain.Arrow,
	constraint string,
) error {
	existing, getErr := s.axArrow.Get(ctx, ns.String())
	if getErr == nil {
		if existing.UserInstalled {
			return nil
		}
		_, sendErr := s.axArrow.SendWait(ctx, arrowcmds.SetUserInstalled{Namespace: ns})
		return sendErr
	}
	if !errors.Is(getErr, asynxModels.ErrNotFound) {
		return fmt.Errorf("add arrow command: %w", getErr)
	}

	cmd := arrowcmds.AddArrow{
		Namespace:           ns,
		ArrowMeta:           arrow.ArrowMeta,
		Variables:           arrow.Variables,
		Netbridge:           arrow.Netbridge,
		Targets:             arrow.Targets,
		Readme:              arrow.Readme,
		DirectInstall:       arrow.UserInstalled,
		InstalledConstraint: constraint,
		RefIsBranch:         arrow.RefIsBranch,
		RefCommitSHA:        arrow.RefCommitSHA,
		Channel:             arrow.Channel,
	}
	_, sendErr := s.axArrow.SendWait(ctx, cmd)
	if sendErr == nil {
		return nil
	}
	if errors.Is(sendErr, asynxModels.ErrValidation) ||
		errors.Is(sendErr, asynxModels.ErrPipelineFailed) {
		return fmt.Errorf("add arrow: %w", apperrors.ErrAlreadyExists)
	}
	return fmt.Errorf("add arrow: %w", sendErr)
}

func (s *arrowService) Remove(
	ctx context.Context,
	ns domain.Namespace,
) error {
	exists, err := s.axArrow.Exists(ctx, ns.String())
	if err != nil {
		return fmt.Errorf("remove: %w", err)
	}
	if !exists {
		return fmt.Errorf("remove: %w", apperrors.ErrNotFound)
	}

	return s.axArrow.Forget(ctx, ns.String())
}

func (s *arrowService) Seed(
	ctx context.Context,
	ns domain.Namespace,
	data []byte,
) error {
	if ns.Validate() != nil {
		return fmt.Errorf("seed arrow: %w", apperrors.ErrInvalidNamespace)
	}
	// Seeded bytes have no remote to ask for a ref, so the caller has to say
	// which one these bytes are.
	if ns.Ref() == "" {
		return fmt.Errorf("seed arrow %s: namespace must carry a ref: %w", ns, apperrors.ErrInvalidNamespace)
	}

	m, err := s.manifold.ParseArrow(data)
	if err != nil {
		return fmt.Errorf("seed arrow: %w: %w", apperrors.ErrInvalidManifest, err)
	}

	// Cacheable, not a bare ManifestFile: seeded bytes are cached like any other
	// manifest, and a cache entry with no index metadata is one the vault lane of
	// search can never answer with.
	if err := s.vault.PutArrow(
		ctx, ns, arrowstore.Cacheable(m, data, "ARROW.md"),
	); err != nil {
		return fmt.Errorf("seed arrow: vault write: %w", err)
	}

	m.UserInstalled = true
	err = s.addArrowCommand(ctx, ns, m, "")
	if err == nil {
		return nil
	}
	if !errors.Is(err, apperrors.ErrAlreadyExists) {
		return fmt.Errorf("seed arrow: %w", err)
	}

	cmd := arrowcmds.UpdateArrowManifest{
		Namespace: ns,
		ArrowMeta: m.ArrowMeta,
		Variables: m.Variables,
		Netbridge: m.Netbridge,
		Targets:   m.Targets,
		Readme:    m.Readme,
	}
	_, err = s.axArrow.SendWait(ctx, cmd)
	return err
}

func (s *arrowService) ValidateManifest(
	ctx context.Context,
	data []byte,
) (*models.ValidationResult, error) {
	m, err := s.manifold.ParseArrow(data)
	if err == nil {
		return validManifestResult(m), nil
	}
	return invalidManifestResult(err), nil
}

// MarkInstalled stays on Send. It is called from inside a runtime projection,
// and waiting there would close the circular wait newAsynx documents
// (internal/app/container.go): an arrow worker blocked on axRuntime for the
// forget cascade while a runtime worker blocks on axArrow for this. Nothing
// reads the install stamp before the runtime reports the arrow ready anyway.
func (s *arrowService) MarkInstalled(
	ctx context.Context,
	ns domain.Namespace,
	at time.Time,
) error {
	_, err := s.axArrow.Send(ctx, arrowcmds.MarkInstalled{
		Namespace:   ns,
		InstalledAt: at,
	})
	return err
}

// MarkUninstalled stays on Send for the same reason MarkInstalled does: it is
// sent from the same runtime projection, so waiting here would close the same
// circular wait.
func (s *arrowService) MarkUninstalled(
	ctx context.Context,
	ns domain.Namespace,
) error {
	_, err := s.axArrow.Send(ctx, arrowcmds.MarkUninstalled{Namespace: ns})
	return err
}

// MarkLastUsed stays on Send for the same reason MarkInstalled does: it is
// sent from the same runtime projection, so waiting here would close the same
// circular wait.
func (s *arrowService) MarkLastUsed(
	ctx context.Context,
	ns domain.Namespace,
	at time.Time,
) error {
	_, err := s.axArrow.Send(ctx, arrowcmds.MarkLastUsed{
		Namespace:  ns,
		LastUsedAt: at,
	})
	return err
}

// SetChannel stays on Send for the same reason MarkInstalled does: it needs
// no wait, and waiting would risk the same circular-wait shape newAsynx
// documents (internal/app/container.go) for other high-frequency commands.
func (s *arrowService) SetChannel(
	ctx context.Context,
	ns domain.Namespace,
	channel string,
	ref string,
) error {
	_, err := s.axArrow.Send(ctx, arrowcmds.SetChannel{
		Namespace: ns,
		Channel:   channel,
		Ref:       ref,
	})
	return err
}

func (s *arrowService) Forget(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return s.axArrow.Forget(ctx, ns.String())
}

// UpdateManifest waits for the projections: a changed manifest changes the
// dependency edges, and the caller reads them back straight away.
func (s *arrowService) UpdateManifest(
	ctx context.Context,
	ns domain.Namespace,
	arrow *domain.Arrow,
) error {
	_, err := s.axArrow.SendWait(ctx, arrowcmds.UpdateArrowManifest{
		Namespace: ns,
		ArrowMeta: arrow.ArrowMeta,
		Variables: arrow.Variables,
		Netbridge: arrow.Netbridge,
		Targets:   arrow.Targets,
		Readme:    arrow.Readme,
	})
	return err
}

func (s *arrowService) Shutdown(ctx context.Context) error {
	return s.axArrow.Shutdown(ctx)
}

// OnArrowAdded registers fn on the projection that owns arrow.added. Callbacks
// run in registration order, before the arrow becomes readable.
func (s *arrowService) OnArrowAdded(
	fn func(ctx context.Context, ns domain.Namespace, arrow domain.Arrow) error,
) error {
	s.callbacksMu.Lock()
	defer s.callbacksMu.Unlock()
	s.addedFns = append(s.addedFns, fn)
	return nil
}

// OnArrowUpdated registers fn on the projection that owns arrow.updated.
func (s *arrowService) OnArrowUpdated(
	fn func(ctx context.Context, ns domain.Namespace, arrow *domain.Arrow) error,
) error {
	s.callbacksMu.Lock()
	defer s.callbacksMu.Unlock()
	s.updatedFns = append(s.updatedFns, fn)
	return nil
}

// OnArrowRemoved registers fn on the forget projection. Callbacks run once the
// read-model row is gone, so nothing can read an arrow whose edges are being
// torn down.
func (s *arrowService) OnArrowRemoved(
	fn func(ctx context.Context, ns domain.Namespace) error,
) error {
	s.callbacksMu.Lock()
	defer s.callbacksMu.Unlock()
	s.removedFns = append(s.removedFns, fn)
	return nil
}

func (s *arrowService) runAdded(
	ctx context.Context,
	arrow domain.Arrow,
) {
	for _, fn := range s.addedCallbacks() {
		if err := fn(ctx, arrow.Namespace, arrow); err != nil {
			slog.ErrorContext(ctx, "arrow callback OnArrowAdded failed",
				"ns", arrow.Namespace, "err", err)
		}
	}
}

func (s *arrowService) runUpdated(
	ctx context.Context,
	arrow domain.Arrow,
) {
	for _, fn := range s.updatedCallbacks() {
		if err := fn(ctx, arrow.Namespace, &arrow); err != nil {
			slog.ErrorContext(ctx, "arrow callback OnArrowUpdated failed",
				"ns", arrow.Namespace, "err", err)
		}
	}
}

func (s *arrowService) runUpgraded(
	ctx context.Context,
	arrow domain.Arrow,
) {
	for _, fn := range s.upgradedCallbacks() {
		if err := fn(ctx, arrow); err != nil {
			slog.ErrorContext(ctx, "arrow callback OnArrowUpgraded failed",
				"ns", arrow.Namespace, "err", err)
		}
	}
}

func (s *arrowService) runRemoved(
	ctx context.Context,
	ns domain.Namespace,
) {
	for _, fn := range s.removedCallbacks() {
		if err := fn(ctx, ns); err != nil {
			slog.ErrorContext(ctx, "arrow callback OnArrowRemoved failed",
				"ns", ns, "err", err)
		}
	}
}

// addedCallbacks and its siblings snapshot the registered callbacks so the
// projection never holds the lock while running them.
func (s *arrowService) addedCallbacks() []func(
	ctx context.Context,
	ns domain.Namespace,
	arrow domain.Arrow,
) error {
	s.callbacksMu.RLock()
	defer s.callbacksMu.RUnlock()
	return slices.Clone(s.addedFns)
}

func (s *arrowService) updatedCallbacks() []func(
	ctx context.Context,
	ns domain.Namespace,
	arrow *domain.Arrow,
) error {
	s.callbacksMu.RLock()
	defer s.callbacksMu.RUnlock()
	return slices.Clone(s.updatedFns)
}

func (s *arrowService) upgradedCallbacks() []func(
	ctx context.Context,
	arrow domain.Arrow,
) error {
	s.callbacksMu.RLock()
	defer s.callbacksMu.RUnlock()
	return slices.Clone(s.upgradedFns)
}

func (s *arrowService) removedCallbacks() []func(
	ctx context.Context,
	ns domain.Namespace,
) error {
	s.callbacksMu.RLock()
	defer s.callbacksMu.RUnlock()
	return slices.Clone(s.removedFns)
}

func (s *arrowService) ResolveLatestStable(
	ctx context.Context,
	ns domain.Namespace,
) (ref string, err error) {
	return s.manifold.ResolveLatestStable(ctx, ns)
}

func (s *arrowService) ResolveConstraint(
	ctx context.Context,
	ns domain.Namespace,
	constraint string,
) (ref string, err error) {
	return s.manifold.ResolveConstraint(ctx, ns, constraint)
}

func (s *arrowService) ResolveTrackedRef(
	ctx context.Context,
	arrow domain.Arrow,
) (ref string, err error) {
	return s.store.ResolveTrackedRef(ctx, arrow)
}

func (s *arrowService) ListChannels(
	ctx context.Context,
	ns domain.Namespace,
) ([]models.ChannelInfo, error) {
	channels, err := s.manifold.ListChannels(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	out := make([]models.ChannelInfo, 0, len(channels))
	for _, c := range channels {
		out = append(out, models.ChannelInfo{
			Name:    c.Name,
			Kind:    c.Kind,
			Latest:  c.Latest,
			Count:   c.Count,
			Members: c.Members,
		})
	}
	return out, nil
}

func (s *arrowService) UpgradeVersion(
	ctx context.Context,
	oldNs domain.Namespace,
	newNs domain.Namespace,
	constraint string,
	channel string,
	runtimeAlreadyExists bool,
	alreadyReady bool,
	userInstalled bool,
	pinnedRef string,
) (*domain.Arrow, error) {
	newArrow, rawBytes, filename, err := s.manifold.ResolveArrow(ctx, newNs)
	if err != nil {
		return nil, fmt.Errorf("upgrade version: fetch manifest: %w", err)
	}

	if !runtimeAlreadyExists { //nolint:nestif
		if delErr := s.vault.DeleteArrow(ctx, newNs); delErr != nil {
			slog.WarnContext(ctx, "upgrade version: delete pre-cached vault entry", "ns", newNs, "err", delErr)
		}
		if err := s.vault.RenameArrow(ctx, oldNs, newNs); err != nil {
			return nil, fmt.Errorf("upgrade version: rename vault entry: %w", err)
		}
		// Cacheable for the same reason Seed uses it: a bare ManifestFile carries
		// no index metadata, so the upgraded ref would be cached on disk yet
		// invisible to the vault lane of search.
		if err := s.vault.PutArrow(
			ctx, newNs, arrowstore.Cacheable(newArrow, rawBytes, filename),
		); err != nil {
			return nil, fmt.Errorf("upgrade version: write new manifest: %w", err)
		}
	}

	if err := s.sendUpgradeArrow(ctx, oldNs, newNs, newArrow, constraint, channel, alreadyReady, userInstalled, pinnedRef); err != nil {
		return nil, err
	}

	return newArrow, nil
}

// UpgradeVersionSeeded parses data locally (no remote fetch), caches it into
// the vault under newNs, and swaps the catalog identity the same way
// UpgradeVersion does. Its one caller today is quiver.core's own
// self-registration: the daemon already holds its embedded self-manifest and
// must never depend on network reachability just to record which version of
// itself is running.
func (s *arrowService) UpgradeVersionSeeded(
	ctx context.Context,
	oldNs domain.Namespace,
	newNs domain.Namespace,
	data []byte,
) error {
	m, err := s.manifold.ParseArrow(data)
	if err != nil {
		return fmt.Errorf("upgrade version seeded: %w: %w", apperrors.ErrInvalidManifest, err)
	}

	if err := s.vault.PutArrow(
		ctx, newNs, arrowstore.Cacheable(m, data, "ARROW.md"),
	); err != nil {
		return fmt.Errorf("upgrade version seeded: vault write: %w", err)
	}

	// Channel is passed empty: UpgradeVersionSeeded's one caller
	// (selfarrow.go) stamps the channel itself via a separate SetChannel
	// call after this returns, which is safe for that caller specifically
	// -- a self-arrow row never carries an InstalledConstraint (self-
	// registration never goes through a glob install), so SetChannel's own
	// unconditional constraint-clear there is a genuine no-op. userInstalled
	// is passed false: self-registration never carries a real UserInstalled
	// fact. pinnedRef is passed empty for the same reason: self-registration
	// never pins to a specific ref.
	return s.sendUpgradeArrow(ctx, oldNs, newNs, m, "", "", true, false, "")
}

// sendUpgradeArrow builds and sends the arrow.upgraded command shared by
// UpgradeVersion and UpgradeVersionSeeded.
//
// Send, not SendWait: the arrow.upgraded projection forgets the old
// namespace (usecases/runtime.go onArrowUpgraded), which is itself a
// blocking send on this same aggregate type. Waiting here would make one
// arrow command depend on another completing.
func (s *arrowService) sendUpgradeArrow(
	ctx context.Context,
	oldNs domain.Namespace,
	newNs domain.Namespace,
	newArrow *domain.Arrow,
	constraint string,
	channel string,
	alreadyReady bool,
	userInstalled bool,
	pinnedRef string,
) error {
	cmd := arrowcmds.UpgradeArrow{
		Namespace:           newNs,
		OldNamespace:        oldNs,
		ArrowMeta:           newArrow.ArrowMeta,
		Variables:           newArrow.Variables,
		Netbridge:           newArrow.Netbridge,
		Targets:             newArrow.Targets,
		Readme:              newArrow.Readme,
		InstalledConstraint: constraint,
		Channel:             channel,
		AlreadyReady:        alreadyReady,
		UserInstalled:       userInstalled,
		PinnedRef:           pinnedRef,
	}
	_, sendErr := s.axArrow.Send(ctx, cmd)
	if sendErr != nil {
		if errors.Is(sendErr, asynxModels.ErrValidation) || errors.Is(sendErr, asynxModels.ErrPipelineFailed) {
			return fmt.Errorf("upgrade version: %w", apperrors.ErrAlreadyExists)
		}
		return fmt.Errorf("upgrade version: send command: %w", sendErr)
	}
	return nil
}

// OnArrowUpgraded registers fn on the projection that owns arrow.upgraded.
func (s *arrowService) OnArrowUpgraded(
	fn func(ctx context.Context, arrow domain.Arrow) error,
) error {
	s.callbacksMu.Lock()
	defer s.callbacksMu.Unlock()
	s.upgradedFns = append(s.upgradedFns, fn)
	return nil
}

func validManifestResult(
	m *domain.Arrow,
) *models.ValidationResult {
	supported := make([]domain.OS, 0, len(m.Targets))
	for os := range m.Targets {
		supported = append(supported, os)
	}
	unsupported := make([]domain.OS, 0)
	for _, os := range domain.AllOS() {
		if _, ok := m.Targets[os]; !ok {
			unsupported = append(unsupported, os)
		}
	}
	return &models.ValidationResult{
		Valid:                true,
		SupportedPlatforms:   supported,
		UnsupportedPlatforms: unsupported,
	}
}

func invalidManifestResult(
	err error,
) *models.ValidationResult {
	var asmErrs ruleset.RuleErrors
	if errors.As(err, &asmErrs) {
		errs := make([]models.ValidationError, len(asmErrs))
		for i, ae := range asmErrs {
			errs[i] = models.ValidationError{
				Field:   ae.Field,
				Rule:    ae.Rule,
				Message: ae.Message,
			}
		}
		return &models.ValidationResult{
			Valid:                false,
			Errors:               errs,
			SupportedPlatforms:   []domain.OS{},
			UnsupportedPlatforms: []domain.OS{},
		}
	}

	return &models.ValidationResult{
		Valid: false,
		Errors: []models.ValidationError{{
			Rule:    "parse_error",
			Message: err.Error(),
		}},
		SupportedPlatforms:   []domain.OS{},
		UnsupportedPlatforms: []domain.OS{},
	}
}
