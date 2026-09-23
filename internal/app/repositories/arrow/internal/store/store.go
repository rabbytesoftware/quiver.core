package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	gormdb "gorm.io/gorm"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store/internal/projections"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store/internal/storage"
	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

const defaultVersionCheckTTL = time.Hour

type ResolveFunc func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error)

type Store interface {
	List(
		ctx context.Context,
		userInstalled *bool,
	) ([]models.ArrowView, error)
	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error)
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
	ResolveForInstall(
		ctx context.Context,
		ns domain.Namespace,
		channel string,
	) (resolvedNs domain.Namespace, arrow *domain.Arrow, constraint string, err error)
	ResolveCatalogued(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.Namespace, error)
	Search(
		ctx context.Context,
		q models.SearchQuery,
	) ([]models.CatalogHit, error)

	// Project makes an arrow readable. The caller decides when, because the
	// order against the reactions that run alongside it is what keeps the read
	// model honest.
	Project(
		ctx context.Context,
		arrow domain.Arrow,
	) error
	// ProjectForget is Project's mirror: it takes the arrow back out of the
	// read model.
	ProjectForget(
		ctx context.Context,
		arrow domain.Arrow,
	) error

	// NeedsVersionCheck claims the TTL slot for a passive version-drift check
	// on ns, atomically, given the last-checked timestamp the caller already
	// has in hand from the same GetDetail read. Staleness is decided in
	// memory first: only when lastCheckedAt already looks due does this fall
	// through to the atomic claim, so the overwhelming majority of calls —
	// well within the TTL — never touch the database at all. It reports false
	// with no error when there is nothing to do: no catalog row for ns, or one
	// checked more recently than the configured TTL — never an error for
	// "nothing to do".
	NeedsVersionCheck(
		ctx context.Context,
		ns domain.Namespace,
		lastCheckedAt time.Time,
	) (bool, error)
	// CheckVersionDrift re-resolves arrow's namespace against the remote and
	// reports whether a better ref exists. ok is false whenever any resolution
	// step errors — the caller must not write a guessed answer in that case.
	CheckVersionDrift(
		ctx context.Context,
		arrow domain.Arrow,
	) (outdated bool, recommendedRef string, ok bool)
	// ResolveTrackedRef resolves the ref arrow should be at right now:
	// constraint-first when InstalledConstraint is set, its tracked channel's
	// latest otherwise (stable when no channel is tracked either). This is
	// the single source of truth CheckVersionDrift's own checkTagDrift
	// already uses, exported so an explicit upgrade (usecases/arrow.go's
	// upgradeRef) resolves its target the identical way the passive
	// drift-check does, rather than a second, independently maintained copy
	// of the same constraint-then-channel rule.
	ResolveTrackedRef(
		ctx context.Context,
		arrow domain.Arrow,
	) (string, error)
}

type storeService struct {
	db              storage.Store
	projector       projections.Projector
	resolveManifest ResolveFunc
	manifold        manifold.Manifold
	platforms       metadata.Platforms
	clock           func() time.Time
	versionCheckTTL time.Duration
}

func New(
	db *gormdb.DB,
	v vault.Vault,
	m manifold.Manifold,
) (Store, error) {
	return newStore(db, v, m, time.Now)
}

// NewWithClock builds a Store whose version-check TTL gating reads the clock
// given instead of time.Now, so a test can control staleness deterministically
// without sleeping.
func NewWithClock(
	db *gormdb.DB,
	v vault.Vault,
	m manifold.Manifold,
	clock func() time.Time,
) (Store, error) {
	return newStore(db, v, m, clock)
}

func newStore(
	db *gormdb.DB,
	v vault.Vault,
	m manifold.Manifold,
	clock func() time.Time,
) (Store, error) {
	st, err := storage.New(db)
	if err != nil {
		return nil, fmt.Errorf("store: storage: %w", err)
	}
	return &storeService{
		db:              st,
		projector:       projections.New(st),
		resolveManifest: newResolver(v, m),
		manifold:        m,
		platforms:       metadata.GetPlatforms(),
		clock:           clock,
		versionCheckTTL: resolveVersionCheckTTL(),
	}, nil
}

func resolveVersionCheckTTL() time.Duration {
	ttl := defaultVersionCheckTTL
	if d, err := time.ParseDuration(config.GetArrows().VersionCheckTTL); err == nil && d > 0 {
		ttl = d
	}
	return ttl
}

func (r *storeService) NeedsVersionCheck(
	ctx context.Context,
	ns domain.Namespace,
	lastCheckedAt time.Time,
) (bool, error) {
	now := r.clock()
	if now.Sub(lastCheckedAt) < r.versionCheckTTL {
		return false, nil
	}
	return r.db.ClaimVersionCheck(ctx, ns, now, r.versionCheckTTL)
}

func (r *storeService) Project(
	ctx context.Context,
	arrow domain.Arrow,
) error {
	return r.projector.Apply(ctx, arrow)
}

func (r *storeService) ProjectForget(
	ctx context.Context,
	arrow domain.Arrow,
) error {
	return r.projector.Forget(ctx, arrow)
}

func (r *storeService) List(
	ctx context.Context,
	userInstalled *bool,
) ([]models.ArrowView, error) {
	vms, err := r.db.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]models.ArrowView, 0, len(vms))
	for _, vm := range vms {
		view, err := r.toArrowView(ctx, vm, userInstalled)
		if err != nil {
			return nil, err
		}
		if view == nil {
			continue
		}
		result = append(result, *view)
	}

	return result, nil
}

func (r *storeService) toArrowView(
	ctx context.Context,
	vm storage.ViewModel,
	userInstalled *bool,
) (*models.ArrowView, error) {
	if userInstalled != nil && hasUserInstalled(vm.Versions) != *userInstalled {
		return nil, nil
	}

	versions, err := r.resolveVersionStates(ctx, vm.Versions)
	if err != nil {
		return nil, err
	}

	return &models.ArrowView{
		Namespace: vm.Namespace,
		Metadata:  vm.Metadata,
		Versions:  versions,
	}, nil
}

func (r *storeService) resolveVersionStates(
	_ context.Context,
	versions []storage.VersionRef,
) ([]models.VersionView, error) {
	result := make([]models.VersionView, 0, len(versions))
	for _, vr := range versions {
		result = append(result, models.VersionView{
			Namespace: vr.Namespace,
			Metadata:  vr.Metadata,
			State:     domain.ArrowStateAbsent,
		})
	}
	return result, nil
}

func (r *storeService) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	vm, err := r.db.FindByKey(ctx, ns.BareNamespace().String())
	if err != nil {
		return nil, fmt.Errorf("reader get: %w", err)
	}
	if vm == nil {
		return nil, apperrors.ErrNotFound
	}
	return &vm.Metadata, nil
}

func (r *storeService) GetDetail(
	ctx context.Context,
	ns domain.Namespace,
) (*models.ArrowDetailView, error) {
	vm, err := r.db.FindByKey(ctx, ns.BareNamespace().String())
	if err != nil {
		return nil, fmt.Errorf("reader get detail: %w", err)
	}
	if vm == nil {
		return r.resolveDetailLive(ctx, ns)
	}

	metadataArrow := vm.Metadata
	lastVersionCheckAt := time.Time{}
	if len(vm.Versions) > 0 {
		lastVersionCheckAt = vm.Versions[0].LastVersionCheckAt
	}

	if ns.Ref() != "" {
		vr, found := findVersionRef(vm.Versions, ns)
		if !found {
			return r.resolveDetailLive(ctx, ns)
		}
		metadataArrow = vr.Metadata
		lastVersionCheckAt = vr.LastVersionCheckAt
	}

	return &models.ArrowDetailView{
		Metadata:           metadataArrow,
		State:              domain.ArrowStateAbsent,
		ActiveRun:          nil,
		LastReturn:         nil,
		LastVersionCheckAt: lastVersionCheckAt,
	}, nil
}

// resolveDetailLive answers GetDetail for a namespace the catalog has no row
// for — either never added at all, or a specific ref never added. It reuses
// ResolveManifest's cascade so an uncatalogued repository previews exactly
// the way GetManifest/GetReadme already resolve it, instead of a bare 404 for
// a namespace that is perfectly resolvable, just not yet installed.
func (r *storeService) resolveDetailLive(
	ctx context.Context,
	ns domain.Namespace,
) (*models.ArrowDetailView, error) {
	arrow, err := r.ResolveManifest(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("reader get detail: %w", err)
	}
	return &models.ArrowDetailView{
		Metadata:   *arrow,
		State:      domain.ArrowStateAbsent,
		ActiveRun:  nil,
		LastReturn: nil,
	}, nil
}

func (r *storeService) GetManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	vm, err := r.db.FindByKey(ctx, ns.BareNamespace().String())
	if err != nil {
		return nil, fmt.Errorf("reader get manifest: %w", err)
	}
	if vm == nil {
		return nil, apperrors.ErrNotFound
	}

	if ns.Ref() == "" {
		return &vm.Metadata, nil
	}

	for _, vr := range vm.Versions {
		if vr.Namespace.String() == ns.String() {
			return &vr.Metadata, nil
		}
	}
	return nil, apperrors.ErrNotFound
}

// ResolveManifest resolves a namespace's manifest, live if the vault has
// never cached it or the cache has gone stale. A ref-less namespace resolves
// to whatever this arrow is already catalogued at, so repeated calls agree
// with the version Add committed to instead of re-guessing "latest" through a
// narrower path than Add itself used. An arrow not yet catalogued falls back
// to the same cascade ResolveForInstall uses to pick a ref for it. The
// returned arrow's Namespace is stamped with whichever ref was actually
// resolved: a manifest declares no version of its own, so manifold parsing
// never sets it.
func (r *storeService) ResolveManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	if ns.Ref() != "" {
		arrow, err := r.resolveManifest(ctx, ns)
		if err != nil {
			return nil, fmt.Errorf("reader resolve manifest: %w", err)
		}
		arrow.Namespace = ns
		return arrow, nil
	}

	arrow, err := r.resolveCatalogedOrLatest(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("reader resolve manifest: %w", err)
	}
	return arrow, nil
}

// ResolveCatalogued maps a namespace as the caller typed it onto the one the
// catalog actually holds it under.
//
// ResolveForInstall guarantees nothing refless ever reaches the catalog, while
// every command accepts a refless namespace. Without this the namespace that
// arrow add has just accepted is rejected by install as not found.
//
// A refless namespace resolves to the preferred version — user-installed
// first, then most recently installed — the same ranking the catalog already
// uses to derive an arrow's own columns. An explicit ref is honoured only if
// the catalog holds it.
func (r *storeService) ResolveCatalogued(
	ctx context.Context,
	ns domain.Namespace,
) (domain.Namespace, error) {
	vm, err := r.db.FindByKey(ctx, ns.BareNamespace().String())
	if err != nil {
		return "", fmt.Errorf("reader resolve catalogued: %w", err)
	}

	if vm == nil || len(vm.Versions) == 0 {
		return "", fmt.Errorf("reader resolve catalogued %s: %w", ns, apperrors.ErrNotFound)
	}

	if ns.Ref() == "" {
		return vm.Versions[0].Namespace, nil
	}

	for _, vr := range vm.Versions {
		if vr.Namespace.String() == ns.String() {
			return ns, nil
		}
	}

	return "", fmt.Errorf("reader resolve catalogued %s: %w", ns, apperrors.ErrNotFound)
}

func (r *storeService) resolveCatalogedOrLatest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	vm, err := r.db.FindByKey(ctx, ns.BareNamespace().String())
	if err != nil {
		return nil, fmt.Errorf("catalog lookup: %w", err)
	}
	if vm == nil {
		resolvedNs, arrow, _, err := r.resolveRefless(ctx, ns, "")
		if err != nil {
			return nil, err
		}
		arrow.Namespace = resolvedNs
		return arrow, nil
	}

	arrow, err := r.resolveManifest(ctx, vm.Metadata.Namespace)
	if err != nil {
		return nil, err
	}
	arrow.Namespace = vm.Metadata.Namespace
	return arrow, nil
}

// ResolveForInstall settles the concrete ref a namespace will live under. A
// glob resolves through its constraint, a refless namespace through the
// requested channel (stable by default), and an explicit ref is taken as
// written. The returned namespace always carries a ref, so nothing refless
// ever reaches the catalog. Every path stamps Channel on the resolved arrow,
// though only the refless path uses channel to drive resolution — the other
// paths derive it from whatever ref they already settled on.
func (r *storeService) ResolveForInstall(
	ctx context.Context,
	ns domain.Namespace,
	channel string,
) (resolvedNs domain.Namespace, arrow *domain.Arrow, constraint string, err error) {
	if ns.IsGlob() {
		return r.resolveGlob(ctx, ns)
	}
	if ns.Ref() == "" {
		return r.resolveRefless(ctx, ns, channel)
	}

	arrow, err = r.resolveManifest(ctx, ns)
	if err != nil {
		return ns, nil, "", fmt.Errorf("reader resolve for install: %w", err)
	}
	if c, ok := manifold.ClassifyChannel(ns.Ref()); ok {
		arrow.Channel = c
	}
	return ns, arrow, "", nil
}

func (r *storeService) resolveGlob(
	ctx context.Context,
	ns domain.Namespace,
) (domain.Namespace, *domain.Arrow, string, error) {
	constraint := ns.Ref()

	resolved, err := r.manifold.ResolveConstraint(ctx, ns, constraint)
	if err != nil {
		return ns, nil, "", fmt.Errorf("reader resolve for install: %w", err)
	}

	resolvedNs := ns.WithRef(resolved)
	arrow, err := r.resolveManifest(ctx, resolvedNs)
	if err != nil {
		return resolvedNs, nil, "", fmt.Errorf("reader resolve for install: %w", err)
	}
	if c, ok := manifold.ClassifyChannel(resolved); ok {
		arrow.Channel = c
	}
	return resolvedNs, arrow, constraint, nil
}

func (r *storeService) resolveRefless(
	ctx context.Context,
	ns domain.Namespace,
	channel string,
) (domain.Namespace, *domain.Arrow, string, error) {
	if channel == "" {
		channel = manifold.StableChannel
	}
	ref, err := r.manifold.ResolveLatestInChannel(ctx, ns, channel)
	if err == nil && ref != "" {
		resolvedNs, arrow, constraint, resolveErr := r.resolveAt(ctx, ns.WithRef(ref))
		if resolveErr != nil {
			return resolvedNs, arrow, constraint, resolveErr
		}
		arrow.Channel = channel
		return resolvedNs, arrow, constraint, nil
	}

	if resolvedNs, arrow, constraint, ok := r.resolveBestOtherChannel(ctx, ns, channel); ok {
		return resolvedNs, arrow, constraint, nil
	}

	return r.resolveDefaultBranch(ctx, ns)
}

func (r *storeService) resolveBestOtherChannel(
	ctx context.Context,
	ns domain.Namespace,
	triedChannel string,
) (domain.Namespace, *domain.Arrow, string, bool) {
	channels, err := r.manifold.ListChannels(ctx, ns)
	if err != nil || len(channels) == 0 {
		return ns, nil, "", false
	}

	for _, candidate := range channels {
		if candidate.Name == triedChannel || candidate.Latest == "" {
			continue
		}

		resolvedNs, arrow, constraint, resolveErr := r.resolveAt(ctx, ns.WithRef(candidate.Latest))
		if resolveErr != nil {
			continue
		}
		arrow.Channel = candidate.Name
		return resolvedNs, arrow, constraint, true
	}

	return ns, nil, "", false
}

// resolveDefaultBranch asks git which branch the repository's HEAD points at.
// That works on every host, so the configured branch list is only reached when
// the remote cannot be listed at all — a raw fetch may still succeed there.
// The resolved arrow is stamped RefIsBranch/RefCommitSHA: this is a mutable
// ref, not a pinned release, and a later version check needs the hash to tell
// whether the branch has since moved. Channel is stamped only when branch is
// itself a genuine, listed channel (see channelIsListed) — a repository that
// actually publishes real channels elsewhere must not have this arrow
// "track" a raw branch snapshot that never appears as an option in its own
// channel listing.
func (r *storeService) resolveDefaultBranch(
	ctx context.Context,
	ns domain.Namespace,
) (domain.Namespace, *domain.Arrow, string, error) {
	branch, hash, err := r.manifold.ResolveDefaultBranch(ctx, ns)
	if err != nil || branch == "" {
		return r.resolveConfiguredBranch(ctx, ns)
	}
	resolvedNs, arrow, constraint, resolveErr := r.resolveAt(ctx, ns.WithRef(branch))
	if resolveErr != nil {
		return resolvedNs, arrow, constraint, resolveErr
	}
	arrow.RefIsBranch = true
	arrow.RefCommitSHA = hash
	if r.channelIsListed(ctx, ns, branch) {
		arrow.Channel = branch
	}
	return resolvedNs, arrow, constraint, nil
}

// resolveConfiguredBranch walks the platform's default branches in order and
// keeps the one that served the manifest: that branch is what the arrow was
// resolved at, so it is the ref the arrow is recorded under. Channel is
// stamped only when the branch is itself a genuine, listed channel — see
// resolveDefaultBranch's own doc comment for why.
func (r *storeService) resolveConfiguredBranch(
	ctx context.Context,
	ns domain.Namespace,
) (domain.Namespace, *domain.Arrow, string, error) {
	branches := r.platforms[ns.Domain()].DefaultBranches
	if len(branches) == 0 {
		return ns, nil, "", fmt.Errorf(
			"reader resolve for install %s: no stable release and no default branch to fall back to: %w",
			ns, apperrors.ErrNotFound,
		)
	}

	var lastErr error
	for _, branch := range branches {
		candidate := ns.WithRef(branch)
		arrow, err := r.resolveManifest(ctx, candidate)
		if err == nil {
			if r.channelIsListed(ctx, ns, branch) {
				arrow.Channel = branch
			}
			return candidate, arrow, "", nil
		}
		lastErr = err
	}

	return ns, nil, "", fmt.Errorf("reader resolve for install: %w", lastErr)
}

// channelIsListed reports whether branch appears as a genuine, selectable
// channel in ns's own ListChannels result — the single source of truth
// ListChannels itself already is (it excludes the default branch whenever
// the repository has any real tag at all), so a raw-branch fallback here
// checks against that same result rather than re-deriving "does this repo
// have tags" a second, separate way and risking the two drifting apart
// again. A ListChannels error means "not confirmed", not "assume
// legitimate": the fallback branch resolution still succeeds, just without
// asserting a channel that couldn't be verified.
func (r *storeService) channelIsListed(
	ctx context.Context,
	ns domain.Namespace,
	branch string,
) bool {
	channels, err := r.manifold.ListChannels(ctx, ns)
	if err != nil {
		return false
	}
	for _, c := range channels {
		if c.Name == branch {
			return true
		}
	}
	return false
}

func (r *storeService) resolveAt(
	ctx context.Context,
	resolvedNs domain.Namespace,
) (domain.Namespace, *domain.Arrow, string, error) {
	arrow, err := r.resolveManifest(ctx, resolvedNs)
	if err != nil {
		return resolvedNs, nil, "", fmt.Errorf("reader resolve for install: %w", err)
	}
	return resolvedNs, arrow, "", nil
}

// Search translates the storage result into the app-layer contract: the
// storage package is internal to this store, so its types cannot cross the
// repository boundary.
func (r *storeService) Search(
	ctx context.Context,
	q models.SearchQuery,
) ([]models.CatalogHit, error) {
	vms, err := r.db.Search(ctx, storage.Query{
		Text:  q.Text,
		OS:    q.OS,
		Limit: q.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("reader search: %w", err)
	}

	hits := make([]models.CatalogHit, 0, len(vms))
	for _, vm := range vms {
		hits = append(hits, models.CatalogHit{
			Namespace:  vm.Namespace,
			Metadata:   vm.Metadata,
			Refs:       refsOf(vm.Versions),
			Provenance: vm.Provenance,
		})
	}
	return hits, nil
}

func refsOf(
	versions []storage.VersionRef,
) []string {
	refs := make([]string, 0, len(versions))
	for _, vr := range versions {
		refs = append(refs, vr.Namespace.Ref())
	}
	return refs
}

// CheckVersionDrift re-resolves arrow's namespace and reports whether a
// better ref exists. The legacy raw branch-hash comparison (checkBranchDrift)
// only applies to a true no-channel branch install: resolveDefaultBranch can
// stamp BOTH RefIsBranch and a genuine, listed Channel on the same row (the
// branch happens to also be a real channel), and once a Channel is set it
// takes priority — checkTagDrift/ResolveTrackedRef is channel- (and
// PinnedRef-) aware, whereas checkBranchDrift only ever compares against the
// repository's default branch, blind to whatever channel or pin the user
// actually chose. A branch-tracked arrow with no channel at all is still
// checked against tags first — once a repository has real tags, a branch is
// never again the answer, however long ago it was resolved onto one.
func (r *storeService) CheckVersionDrift(
	ctx context.Context,
	arrow domain.Arrow,
) (bool, string, bool) {
	if arrow.RefIsBranch && arrow.Channel == "" {
		return r.checkBranchDrift(ctx, arrow)
	}
	return r.checkTagDrift(ctx, arrow)
}

func (r *storeService) checkBranchDrift(
	ctx context.Context,
	arrow domain.Arrow,
) (bool, string, bool) {
	latestTag, err := r.manifold.ResolveLatestStable(ctx, arrow.Namespace)
	if err == nil && latestTag != "" {
		return true, latestTag, true
	}
	if err != nil && !errors.Is(err, manifold.ErrNoLatestStable) {
		return false, "", false
	}

	branch, hash, err := r.manifold.ResolveDefaultBranch(ctx, arrow.Namespace)
	if err != nil {
		return false, "", false
	}
	if branch != arrow.Namespace.Ref() || hash != arrow.RefCommitSHA {
		return true, "", true
	}
	return false, "", true
}

func (r *storeService) checkTagDrift(
	ctx context.Context,
	arrow domain.Arrow,
) (bool, string, bool) {
	latest, err := r.ResolveTrackedRef(ctx, arrow)
	if err != nil {
		return false, "", false
	}
	if latest != arrow.Namespace.Ref() {
		return true, latest, true
	}
	return false, "", true
}

func (r *storeService) ResolveTrackedRef(
	ctx context.Context,
	arrow domain.Arrow,
) (string, error) {
	if arrow.InstalledConstraint != "" {
		return r.manifold.ResolveConstraint(ctx, arrow.Namespace, arrow.InstalledConstraint)
	}
	if arrow.PinnedRef != "" {
		return arrow.PinnedRef, nil
	}
	return r.manifold.ResolveLatestInChannel(ctx, arrow.Namespace, channelOf(arrow))
}

func channelOf(
	arrow domain.Arrow,
) string {
	if arrow.Channel == "" {
		return manifold.StableChannel
	}
	return arrow.Channel
}

func findVersionRef(
	versions []storage.VersionRef,
	ns domain.Namespace,
) (storage.VersionRef, bool) {
	for _, vr := range versions {
		if vr.Namespace.String() == ns.String() {
			return vr, true
		}
	}
	return storage.VersionRef{}, false
}

func hasUserInstalled(
	versions []storage.VersionRef,
) bool {
	for _, v := range versions {
		if v.Metadata.UserInstalled {
			return true
		}
	}
	return false
}
