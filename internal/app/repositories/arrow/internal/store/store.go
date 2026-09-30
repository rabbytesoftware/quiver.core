package store

import (
	"context"
	"fmt"
	"time"

	gormdb "gorm.io/gorm"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store/internal/projections"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store/internal/storage"
	"github.com/rabbytesoftware/quiver.core/internal/core/config"
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
	// ResolveInstall settles the identity a namespace is installed under and
	// what that identity resolves to right now: a refless namespace follows
	// its repository's default channel, any other keeps its ref as the
	// selector. The manifest is the one at the resolved commit; it is cached
	// only when CacheWhenAbsent is given and the identity has no row yet.
	ResolveInstall(
		ctx context.Context,
		ns domain.Namespace,
		opts ...InstallOption,
	) (identity domain.Namespace, arrow *domain.Arrow, err error)
	// CheckDrift reports what arrow's selector points at when that differs
	// from what arrow has installed, nil when it is current. ok is false
	// whenever the remote could not answer, and the caller must then record
	// nothing. It reads the remote live, bypassing the snapshot cache: its
	// callers are already rate-limited to one check per version-check TTL.
	CheckDrift(
		ctx context.Context,
		arrow domain.Arrow,
	) (available *domain.Available, ok bool)
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
}

type storeService struct {
	db              storage.Store
	projector       projections.Projector
	resolveManifest ResolveFunc
	vault           vault.Vault
	manifold        manifold.Manifold
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
		vault:           v,
		manifold:        m,
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
	r.classifyPreview(ctx, arrow)
	return &models.ArrowDetailView{
		Metadata:   *arrow,
		State:      domain.ArrowStateAbsent,
		ActiveRun:  nil,
		LastReturn: nil,
	}, nil
}

// classifyPreview names the selector kind an add of the previewed namespace
// would record. A manifest read straight at a ref never classified it, and
// the zero kind would call every selector a pin. It is best-effort: a remote
// that cannot be listed leaves the preview as resolved.
func (r *storeService) classifyPreview(
	ctx context.Context,
	arrow *domain.Arrow,
) {
	selector := arrow.Namespace.Ref()
	if r.manifold == nil || selector == "" || arrow.Resolved.Commit != "" {
		return
	}
	snap, err := r.manifold.Snapshot(ctx, arrow.Namespace)
	if err != nil {
		return
	}
	kind, err := manifold.ClassifySelector(selector, snap)
	if err != nil {
		return
	}
	arrow.SelectorKind = kind
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
// with the version Add committed to. An arrow not yet catalogued resolves the
// way Add would install it. The
// returned arrow's Namespace is stamped with whichever ref was actually
// resolved: a manifest declares no version of its own, so manifold parsing
// never sets it.
func (r *storeService) ResolveManifest(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	if ns.Ref() != "" {
		arrow, err := r.resolveAtRef(ctx, ns)
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

// resolveAtRef falls back to reading a selector identity (pkg@v1.*,
// pkg@stable) at its target commit, since no host serves a selector as a
// ref. When that fails too, the original failure is the one that describes ns.
func (r *storeService) resolveAtRef(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error) {
	arrow, err := r.resolveManifest(ctx, ns)
	if err == nil || r.manifold == nil {
		return arrow, err
	}
	_, selected, selErr := r.ResolveInstall(ctx, ns)
	if selErr != nil {
		return nil, err
	}
	return selected, nil
}

// ResolveCatalogued maps a namespace as the caller typed it onto the one the
// catalog actually holds it under.
//
// ResolveInstall guarantees nothing refless ever reaches the catalog, while
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
		identity, arrow, err := r.ResolveInstall(ctx, ns)
		if err != nil {
			return nil, err
		}
		arrow.Namespace = identity
		return arrow, nil
	}

	arrow, err := r.resolveManifest(ctx, vm.Metadata.Namespace)
	if err != nil {
		return nil, err
	}
	arrow.Namespace = vm.Metadata.Namespace
	return arrow, nil
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
