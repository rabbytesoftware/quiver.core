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
	) (resolvedNs domain.Namespace, arrow *domain.Arrow, constraint string, err error)
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
	// on ns, atomically. It reports false with no error when there is nothing
	// to do: no catalog row for ns, or one checked more recently than the
	// configured TTL — never an error for "nothing to do".
	NeedsVersionCheck(
		ctx context.Context,
		ns domain.Namespace,
	) (bool, error)
	// CheckVersionDrift re-resolves arrow's namespace against the remote and
	// reports whether a better ref exists. ok is false whenever any resolution
	// step errors — the caller must not write a guessed answer in that case.
	CheckVersionDrift(
		ctx context.Context,
		arrow domain.Arrow,
	) (outdated bool, recommendedRef string, ok bool)
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
) (bool, error) {
	return r.db.ClaimVersionCheck(ctx, ns, r.clock(), r.versionCheckTTL)
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

	if ns.Ref() != "" {
		vr, found := findVersionRef(vm.Versions, ns)
		if !found {
			return r.resolveDetailLive(ctx, ns)
		}
		metadataArrow = vr.Metadata
	}

	return &models.ArrowDetailView{
		Metadata:   metadataArrow,
		State:      domain.ArrowStateAbsent,
		ActiveRun:  nil,
		LastReturn: nil,
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

func (r *storeService) resolveCatalogedOrLatest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	vm, err := r.db.FindByKey(ctx, ns.BareNamespace().String())
	if err != nil {
		return nil, fmt.Errorf("catalog lookup: %w", err)
	}
	if vm == nil {
		resolvedNs, arrow, _, err := r.resolveRefless(ctx, ns)
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
// glob resolves through its constraint, a refless namespace through the latest
// stable release, and an explicit ref is taken as written. The returned
// namespace always carries a ref, so nothing refless ever reaches the catalog.
func (r *storeService) ResolveForInstall(
	ctx context.Context,
	ns domain.Namespace,
) (resolvedNs domain.Namespace, arrow *domain.Arrow, constraint string, err error) {
	if ns.IsGlob() {
		return r.resolveGlob(ctx, ns)
	}
	if ns.Ref() == "" {
		return r.resolveRefless(ctx, ns)
	}

	arrow, err = r.resolveManifest(ctx, ns)
	if err != nil {
		return ns, nil, "", fmt.Errorf("reader resolve for install: %w", err)
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
	return resolvedNs, arrow, constraint, nil
}

// resolveRefless reads a refless namespace as "the latest stable release", and
// a repository that publishes none as "whatever its default branch is". Both
// answers come from the remote, so both are facts and both are committed to.
func (r *storeService) resolveRefless(
	ctx context.Context,
	ns domain.Namespace,
) (domain.Namespace, *domain.Arrow, string, error) {
	ref, err := r.manifold.ResolveLatestStable(ctx, ns)
	if err != nil || ref == "" {
		return r.resolveDefaultBranch(ctx, ns)
	}
	return r.resolveAt(ctx, ns.WithRef(ref))
}

// resolveDefaultBranch asks git which branch the repository's HEAD points at.
// That works on every host, so the configured branch list is only reached when
// the remote cannot be listed at all — a raw fetch may still succeed there.
// The resolved arrow is stamped RefIsBranch/RefCommitSHA: this is a mutable
// ref, not a pinned release, and a later version check needs the hash to tell
// whether the branch has since moved.
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
	return resolvedNs, arrow, constraint, nil
}

// resolveConfiguredBranch walks the platform's default branches in order and
// keeps the one that served the manifest: that branch is what the arrow was
// resolved at, so it is the ref the arrow is recorded under.
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
			return candidate, arrow, "", nil
		}
		lastErr = err
	}

	return ns, nil, "", fmt.Errorf("reader resolve for install: %w", lastErr)
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
// better ref exists. A branch-tracked arrow is checked against tags first —
// once a repository has real tags, a branch is never again the answer,
// however long ago it was resolved onto one.
func (r *storeService) CheckVersionDrift(
	ctx context.Context,
	arrow domain.Arrow,
) (bool, string, bool) {
	if arrow.RefIsBranch {
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
	var latest string
	var err error
	if arrow.InstalledConstraint != "" {
		latest, err = r.manifold.ResolveConstraint(ctx, arrow.Namespace, arrow.InstalledConstraint)
	} else {
		latest, err = r.manifold.ResolveLatestStable(ctx, arrow.Namespace)
	}
	if err != nil {
		return false, "", false
	}
	if latest != arrow.Namespace.Ref() {
		return true, latest, true
	}
	return false, "", true
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
