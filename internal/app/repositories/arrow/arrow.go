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
	) error
	Remove(
		ctx context.Context,
		ns domain.Namespace,
	) error
	// Advance moves ns's row in place to target, refreshing its cached
	// manifest to the one at target's commit.
	Advance(
		ctx context.Context,
		ns domain.Namespace,
		target domain.Available,
	) error
	// CheckAvailable re-resolves ns against a live snapshot, records what is
	// ahead of it as its Available (nil when current) and returns it.
	CheckAvailable(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Available, error)
	// TargetUnmoved reports whether target's ref still stands at target's
	// commit on the remote right now.
	TargetUnmoved(
		ctx context.Context,
		ns domain.Namespace,
		target domain.Available,
	) (bool, error)
	// RefreshToTarget stages the manifest at target's commit on ns's row,
	// leaving what is installed untouched, and returns it.
	RefreshToTarget(
		ctx context.Context,
		ns domain.Namespace,
		target domain.Available,
	) (*domain.Arrow, error)
	// AddDependency catalogues the row a dependency declaration installs, if
	// it is not catalogued yet, and returns its identity.
	AddDependency(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.Namespace, error)
	// Adopt registers already-installed state for ns from manifest bytes the
	// caller holds, named filename, creating the row or advancing it, without
	// any network.
	Adopt(
		ctx context.Context,
		ns domain.Namespace,
		kind domain.SelectorKind,
		resolved domain.Resolved,
		manifest []byte,
		filename string,
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
	// CheckVersionNow launches an immediate version-drift check for ns,
	// bypassing the TTL GetDetail's passive maybeCheckVersion otherwise
	// enforces, detached from the caller.
	CheckVersionNow(
		ctx context.Context,
		ns domain.Namespace,
	)
	Forget(
		ctx context.Context,
		ns domain.Namespace,
	) error
	// ListChannels reports every channel ns's repository publishes.
	ListChannels(
		ctx context.Context,
		ns domain.Namespace,
	) ([]models.ChannelInfo, error)
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
		{"arrow.advanced.*", s.projectUpdated},
		{"arrow.manifest_refreshed.*", s.projectUpdated},
		{"arrow.installed.*", s.projectInstallStamp},
		{"arrow.uninstalled.*", s.projectInstallStamp},
		{"arrow.available_checked.*", s.projectVersionCheck},
		{"arrow.user_installed.*", s.projectUsage},
		{"arrow.last_used.*", s.projectUsage},
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

// projectInstallStamp carries the installed-ref stamp into the read model — set
// by an install, cleared by an uninstall. Nothing derived hangs off it, so there
// is no reaction to run first.
func (s *arrowService) projectInstallStamp(
	ctx context.Context,
	evt asynxModels.Event[domain.Arrow],
) {
	s.project(ctx, evt.Aggregate, nil)
}

// projectVersionCheck carries the available stamp into the read model. Nothing derived hangs off it, so there is no reaction to run
// first — same shape as projectInstallStamp.
func (s *arrowService) projectVersionCheck(
	ctx context.Context,
	evt asynxModels.Event[domain.Arrow],
) {
	s.project(ctx, evt.Aggregate, nil)
}

// projectUsage carries the user-installed flag and the last-used stamp into
// the read model, which the library's user_installed filter reads.
func (s *arrowService) projectUsage(
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
// trustworthy "current" than it is a drift.
//
// The runtime state is reconciled unconditionally: the catalog record is only
// written when the answer changed, so a runtime badge that diverged from an
// unchanged answer would otherwise never heal.
func (s *arrowService) runVersionCheck(
	ctx context.Context,
	arrow domain.Arrow,
) {
	available, answered, err := s.recordAvailable(ctx, arrow.Namespace,
		func(current domain.Arrow) (*domain.Available, bool, error) {
			available, ok := s.store.CheckDrift(ctx, current)
			return available, ok, nil
		})
	if !answered {
		return
	}
	if err != nil {
		slog.WarnContext(ctx, "arrow version check: record", "ns", arrow.Namespace, "err", err)
	}
	s.syncVersionOutdated(ctx, arrow.Namespace, available != nil)
}

// maxWriteAttempts bounds how often a write to a row another writer changed
// under it is judged or sent again.
const maxWriteAttempts = 3

// availableFn judges what is ahead of current; answered is false when it has
// no trustworthy answer.
type availableFn func(current domain.Arrow) (available *domain.Available, answered bool, err error)

// recordAvailable records judge's answer about ns's row. The answer is
// written only while the row still holds the Resolved it was judged against:
// the command refuses it otherwise, and a concurrent append to the row fails
// it with a version conflict. Either way the row is re-read and judged
// again, a bounded number of times, so an answer about a Resolved the row has
// already left (a row outdated right after its update) is never recorded.
func (s *arrowService) recordAvailable(
	ctx context.Context,
	ns domain.Namespace,
	judge availableFn,
) (*domain.Available, bool, error) {
	for attempt := 1; ; attempt++ {
		current, err := s.axArrow.Get(ctx, ns.String())
		if err != nil {
			return nil, false, err
		}
		available, answered, err := judge(current)
		if err != nil || !answered {
			return nil, answered, err
		}
		if sameAvailable(current.Available, available) {
			return available, true, nil
		}

		_, err = s.axArrow.SendWait(ctx, arrowcmds.RecordAvailable{
			Namespace:      ns,
			Available:      available,
			JudgedResolved: current.Resolved,
		})
		if err == nil {
			return available, true, nil
		}
		if !retryableWrite(err) || attempt == maxWriteAttempts {
			return available, true, err
		}
	}
}

// retryableWrite reports whether a rejected write is worth judging again:
// the row changed under it, either by a concurrent append or before a
// command's own check against what the writer read.
func retryableWrite(err error) bool {
	return errors.Is(err, asynxModels.ErrPipelineFailed) || errors.Is(err, asynxModels.ErrValidation)
}

func sameAvailable(
	a *domain.Available,
	b *domain.Available,
) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// CheckVersionNow launches a version-drift check for ns immediately,
// bypassing NeedsVersionCheck's TTL claim. The row comes from axArrow, not the
// read model: a command that just changed the row has reached the aggregate
// before its projection has reached the read model. A namespace with no row is
// a no-op.
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
) error {
	identity, arrow, err := s.store.ResolveInstall(ctx, ns, arrowstore.CacheWhenAbsent(s.identityExists))
	if err != nil {
		return fmt.Errorf("add: %w", mapResolveErr(err))
	}
	arrow.UserInstalled = true
	if err := s.markIfPreinstalled(ctx, identity, arrow); err != nil {
		return err
	}
	return s.addArrowCommand(ctx, identity, arrow)
}

func (s *arrowService) identityExists(
	ctx context.Context,
	identity domain.Namespace,
) (bool, error) {
	return s.axArrow.Exists(ctx, identity.String())
}

// addArrowCommand waits for the projections rather than only for the write.
// The caller's next move is to install the arrow, and installing reads the
// dependency edges this event produces; returning before they exist is what
// lets a still-depended-on arrow be removed.
func (s *arrowService) addArrowCommand(
	ctx context.Context,
	ns domain.Namespace,
	arrow *domain.Arrow,
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
		Namespace:     ns,
		ArrowMeta:     arrow.ArrowMeta,
		Variables:     arrow.Variables,
		Netbridge:     arrow.Netbridge,
		Targets:       arrow.Targets,
		Readme:        arrow.Readme,
		DirectInstall: arrow.UserInstalled,
		SelectorKind:  arrow.SelectorKind,
		Resolved:      arrow.Resolved,
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

func (s *arrowService) Forget(
	ctx context.Context,
	ns domain.Namespace,
) error {
	return s.axArrow.Forget(ctx, ns.String())
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

func (s *arrowService) removedCallbacks() []func(
	ctx context.Context,
	ns domain.Namespace,
) error {
	s.callbacksMu.RLock()
	defer s.callbacksMu.RUnlock()
	return slices.Clone(s.removedFns)
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
