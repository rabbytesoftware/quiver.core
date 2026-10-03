package manifold

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/compiler"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
	resolvers "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver/resolvers"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/translator"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/versioning"
)

// Manifold resolves arrow and quiver manifests from remote git repositories.
// Given a namespace it fetches the YAML manifest, validates and parses it,
// then validates the result with business rules.
type Manifold interface {
	// ResolveArrow fetches and validates an ArrowManifest for the given namespace.
	// The returned aggregate includes compiled OS-specific targets in manifest.Targets.
	// Also returns the raw manifest bytes and the filename it was resolved from.
	// The returned arrow carries no Namespace, except when Fletcher drafted it
	// from a ref other than the one namespace named (a branch whose release
	// assets live under a tag): then Namespace is the bare namespace at the
	// ref actually drafted, which is the revision the arrow really is.
	ResolveArrow(
		ctx context.Context,
		namespace domain.Namespace,
	) (*domain.Arrow, []byte, string, error)

	// ResolveArrowAt fetches and validates an ArrowManifest at an explicit path
	// within its repository, skipping the owning-collection lookup ResolveArrow
	// performs for a quiver-hosted namespace. Use this when the caller already
	// knows the arrow's location — e.g. Follow, which just derived it from the
	// collection's own arrow list — to avoid re-resolving that same collection
	// once per local arrow.
	ResolveArrowAt(
		ctx context.Context,
		namespace domain.Namespace,
		path string,
	) (*domain.Arrow, []byte, string, error)

	// ResolveCollection fetches and validates a Quiver for the given namespace.
	ResolveCollection(
		ctx context.Context,
		namespace domain.Namespace,
	) (*domain.Collection, error)

	// ParseCollection translates and validates a raw quiver manifest (YAML or QUIVER.md bytes)
	// without fetching from a remote source. Derives local arrow namespaces from ns.
	ParseCollection(
		data []byte,
		ns domain.Namespace,
	) (*domain.Collection, error)

	// ParseArrow translates and validates a raw YAML arrow manifest without
	// fetching from a remote source. Returns RuleErrors if validation fails.
	ParseArrow(
		data []byte,
	) (*domain.Arrow, error)

	// ListChannels buckets every tag a namespace's repository publishes
	// into its channel. The repository's default branch is included as one
	// more pointer channel only when the repository has no tags at all —
	// not merely no ordered channels, genuinely zero tags of any kind,
	// ordered or pointer: a repository that has cut even a single
	// non-version tag already has a real channel to offer, so the moving
	// default branch is never listed alongside it. It never fails just
	// because the repository has no default branch — that only shrinks the
	// result.
	ListChannels(
		ctx context.Context,
		ns domain.Namespace,
	) ([]ChannelInfo, error)

	// Snapshot reads every tag, branch and the HEAD branch of ns's
	// repository in one round trip, cached for the manifold's cache TTL.
	Snapshot(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.RefSnapshot, error)

	// FreshSnapshot is Snapshot read live from the remote, refreshing the
	// cache: for a decision that must not act on a view up to a TTL old.
	FreshSnapshot(
		ctx context.Context,
		ns domain.Namespace,
	) (domain.RefSnapshot, error)

	// ResolveArrowAtCommit fetches ns's manifest at commit, the commit ref
	// resolved to, and returns it stamped with ns itself. Raw-file hosts serve
	// a commit SHA as a ref; the clone path only checks out tags and branches,
	// so when the fetch at the commit fails it falls back to ref — never to
	// ns's own selector, which need not name a git ref at all — and the caller
	// verifies the commit afterwards. A manifest that fetched but is invalid
	// never falls back.
	ResolveArrowAtCommit(
		ctx context.Context,
		ns domain.Namespace,
		ref string,
		commit string,
	) (*domain.Arrow, []byte, string, error)

	// ResolveReleaseAsset names the asset of the release ns's ref publishes
	// that os runs, with the SHA-256 digest its host published for it. The
	// failures are ErrNoRelease, ErrUnsupportedPlatform, ErrNoAsset and
	// ErrUnverifiable; a host that could not be asked returns its own error.
	ResolveReleaseAsset(
		ctx context.Context,
		ns domain.Namespace,
		os domain.OS,
	) (domain.ReleaseAsset, error)
}

// ErrInvalidManifest reports that manifest content — fetched or handed in
// directly — failed to become a valid domain.Arrow: bad YAML, a ruleset
// violation, or a compile/post-compile validation failure. It wraps every
// error ParseArrow returns, so a caller can tell "the content is bad" apart
// from a resolver-layer fetch failure without inspecting error text.
var ErrInvalidManifest = errors.New("manifold: invalid manifest")

// ErrArrowNotInCollection reports that a quiver-hosted namespace's AUID has
// no matching entry in its owning collection's current arrow list.
var ErrArrowNotInCollection = errors.New("manifold: arrow not found in its collection")

// StableChannel is the channel a tag belongs to when it carries no channel
// suffix at all.
const StableChannel = resolvers.StableChannel

// ChannelInfo describes one channel a namespace's repository publishes.
type ChannelInfo = models.ChannelInfo

// defaultManifoldCacheTTL is the fallback used when a Manifold is built with
// no explicit cache TTL (a zero/negative value passed to New, or
// NewWithResolvers's own test/harness construction path). It must equal
// internal/app/repositories/arrow/internal/store/store.go's own
// defaultVersionCheckTTL: production wiring (internal/engine/container.go)
// ties the real cache TTL to the SAME config.GetArrows().VersionCheckTTL
// value that store.go's drift-check throttle already reads, specifically so
// the two can never silently drift apart — an hour-scale "is this arrow due
// for a recheck" throttle and a cache with a longer staleness window would
// otherwise let a cache hit silently swallow a check the throttle just said
// was due. This constant is only the shared fallback both layers already
// agree on when config supplies nothing usable; it is not itself the
// mechanism that keeps them in sync — the shared config value is.
const defaultManifoldCacheTTL = time.Hour

type manifold struct {
	rsv       resolver.Resolver
	trs       translator.Translator
	cmp       compiler.Compiler
	rls       ruleset.Ruleset
	hosts     HostLookup
	timeout   time.Duration
	fl        fletcher.Fletcher
	snapshots versioning.Snapshots
}

// New builds a Manifold that asks lookup whatever only a git host can answer.
// A nil lookup is a manifold that knows no hosts, which resolves every
// namespace by cloning it. cacheTTL bounds the Snapshot/ListChannels cache; a zero or negative value falls back to defaultManifoldCacheTTL.
// Callers should derive cacheTTL from config.GetArrows().VersionCheckTTL
// (see internal/engine/container.go) so this cache's staleness window can
// never outlive the drift-check throttle that value already governs.
func New(
	fetchTimeout time.Duration,
	lookup HostLookup,
	cacheTTL time.Duration,
	opts ...Option,
) Manifold {
	return newManifold(fetchTimeout, lookup, resolvers.NewConstraintResolver(fetchTimeout), cacheTTL, time.Now, opts)
}

// NewWithClock is New with an injectable clock, so a test can advance time
// deterministically past cacheTTL — including the real, config-derived
// production default — without a real sleep. Its one caller today is the
// integration test harness (tests/kit); production wiring always uses New.
func NewWithClock(
	fetchTimeout time.Duration,
	lookup HostLookup,
	cacheTTL time.Duration,
	clock func() time.Time,
	opts ...Option,
) Manifold {
	return newManifold(fetchTimeout, lookup, resolvers.NewConstraintResolver(fetchTimeout), cacheTTL, clock, opts)
}

func newManifold(
	fetchTimeout time.Duration,
	lookup HostLookup,
	crs resolvers.ConstraintResolver,
	cacheTTL time.Duration,
	clock func() time.Time,
	opts []Option,
) Manifold {
	lookup = hosts.Or(lookup)
	if cacheTTL <= 0 {
		cacheTTL = defaultManifoldCacheTTL
	}

	return withOptions(&manifold{
		rsv:       resolver.New(fetchTimeout, lookup),
		trs:       translator.NewTranslator(),
		cmp:       compiler.New(),
		rls:       ruleset.New(),
		hosts:     lookup,
		timeout:   fetchTimeout,
		snapshots: versioning.New(crs, clock, cacheTTL),
	}, opts)
}

// NewWithResolvers builds a Manifold with an injected resolver, constraint
// resolver and host lookup. Intended for tests that need to control how
// namespaces are resolved; its cache TTL is always defaultManifoldCacheTTL,
// with the real clock, since no caller of this constructor previously
// needed either different — see NewWithResolversAndClock for the one that
// does.
func NewWithResolvers(
	rsv resolver.Resolver,
	crs resolvers.ConstraintResolver,
	lookup HostLookup,
	opts ...Option,
) Manifold {
	return NewWithResolversAndClock(rsv, crs, lookup, time.Now, opts...)
}

// NewWithResolversAndClock is NewWithResolvers with an injectable clock, so
// the integration test harness (tests/kit) can advance time deterministically
// past cacheTTL — including the real, config-derived production default —
// without a real sleep, the same way NewWithClock does for New.
func NewWithResolversAndClock(
	rsv resolver.Resolver,
	crs resolvers.ConstraintResolver,
	lookup HostLookup,
	clock func() time.Time,
	opts ...Option,
) Manifold {
	return withOptions(&manifold{
		rsv:       rsv,
		trs:       translator.NewTranslator(),
		cmp:       compiler.New(),
		rls:       ruleset.New(),
		hosts:     hosts.Or(lookup),
		snapshots: versioning.New(crs, clock, defaultManifoldCacheTTL),
	}, opts)
}

func withOptions(
	m *manifold,
	opts []Option,
) Manifold {
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *manifold) ResolveArrow(
	ctx context.Context,
	namespace domain.Namespace,
) (*domain.Arrow, []byte, string, error) {
	raw, filename, err := m.resolveArrowBytes(ctx, namespace)
	draftedFrom := ""
	if err != nil && m.fl != nil {
		raw, filename, draftedFrom, err = m.fl.Recover(ctx, namespace, err)
	}
	if err != nil {
		return nil, nil, "", err
	}

	arrow, err := m.ParseArrow(raw)
	if err != nil {
		return nil, nil, "", err
	}
	if draftedFrom != "" {
		arrow.Namespace = namespace.WithRef(draftedFrom)
	}

	return arrow, raw, filename, nil
}

func (m *manifold) ResolveArrowAt(
	ctx context.Context,
	namespace domain.Namespace,
	path string,
) (*domain.Arrow, []byte, string, error) {
	raw, filename, err := m.rsv.ResolveArrowAt(ctx, namespace, path)
	if err != nil {
		return nil, nil, "", err
	}

	arrow, err := m.ParseArrow(raw)
	if err != nil {
		return nil, nil, "", err
	}

	return arrow, raw, filename, nil
}

// resolveArrowBytes fetches an arrow manifest's raw bytes. A quiver-hosted
// (4-segment) namespace has no fixed on-disk location: its file can live
// anywhere in its owning collection's repository, so the owning collection
// is resolved first and its AUID looked up there. Every other namespace
// keeps the flat ARROW.md/arrow.yaml-at-root lookup.
func (m *manifold) resolveArrowBytes(
	ctx context.Context,
	namespace domain.Namespace,
) ([]byte, string, error) {
	if !namespace.BareNamespace().IsQuiverHosted() {
		return m.rsv.ResolveArrow(ctx, namespace)
	}

	path, err := m.resolveLocalArrowPath(ctx, namespace)
	if err != nil {
		return nil, "", err
	}
	return m.rsv.ResolveArrowAt(ctx, namespace, path)
}

func (m *manifold) resolveLocalArrowPath(
	ctx context.Context,
	namespace domain.Namespace,
) (string, error) {
	bare := namespace.BareNamespace()
	quid := domain.Namespace(bare.GetQUID()).WithRef(namespace.Ref())

	coll, err := m.ResolveCollection(ctx, quid)
	if err != nil {
		return "", fmt.Errorf("manifold: resolve owning collection %s for arrow %s: %w", quid, namespace, err)
	}

	for _, a := range coll.Arrows {
		if a.IsLocal && a.Namespace.BareNamespace() == bare {
			return a.SourcePath, nil
		}
	}

	return "", fmt.Errorf("manifold: arrow %s: %w", namespace, ErrArrowNotInCollection)
}

func (m *manifold) ParseArrow(
	data []byte,
) (*domain.Arrow, error) {
	module, err := m.trs.Arrow(data)
	if err != nil {
		return nil, fmt.Errorf("manifold: parse arrow: %w: %w", ErrInvalidManifest, err)
	}

	if readme, ok := m.trs.ExtractReadme(data); ok {
		module.Manifest.Readme = readme
	}

	if err := m.rls.ValidatePrecompile(module.Manifest, module.Precompiled); err != nil {
		return nil, fmt.Errorf("manifold: parse arrow: %w: %w", ErrInvalidManifest, err)
	}

	if err := m.cmp.Compile(module.Manifest, module.Precompiled, module.Selector); err != nil {
		return nil, fmt.Errorf("manifold: parse arrow: %w: %w", ErrInvalidManifest, err)
	}

	if err := m.rls.ValidateCompiled(module.Manifest); err != nil {
		return nil, fmt.Errorf("manifold: parse arrow: %w: %w", ErrInvalidManifest, err)
	}

	return module.Manifest, nil
}

func (m *manifold) ListChannels(
	ctx context.Context,
	ns domain.Namespace,
) ([]ChannelInfo, error) {
	snap, err := m.Snapshot(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("manifold: list channels: %w", err)
	}
	return versioning.ChannelsOf(snap), nil
}

func (m *manifold) Snapshot(
	ctx context.Context,
	ns domain.Namespace,
) (domain.RefSnapshot, error) {
	return m.snapshots.Snapshot(ctx, ns)
}

func (m *manifold) FreshSnapshot(
	ctx context.Context,
	ns domain.Namespace,
) (domain.RefSnapshot, error) {
	return m.snapshots.FreshSnapshot(ctx, ns)
}

func (m *manifold) ResolveLatestStable(
	ctx context.Context,
	ns domain.Namespace,
) (string, error) {
	snap, err := m.Snapshot(ctx, ns)
	if err != nil {
		return "", fmt.Errorf("manifold: latest stable: %w", err)
	}
	latest, ok := versioning.LatestStable(snap)
	if !ok {
		return "", fmt.Errorf("manifold: latest stable %s: %w", ns, models.ErrNoLatestStable)
	}
	return latest, nil
}

func (m *manifold) ResolveDefaultBranch(
	ctx context.Context,
	ns domain.Namespace,
) (branch, hash string, err error) {
	snap, err := m.Snapshot(ctx, ns)
	if err != nil {
		return "", "", fmt.Errorf("manifold: default branch: %w", err)
	}
	branch, hash, ok := versioning.DefaultBranch(snap)
	if !ok {
		return "", "", fmt.Errorf("manifold: default branch %s: %w", ns, versioning.ErrUnknownSelector)
	}
	return branch, hash, nil
}

func (m *manifold) ResolveArrowAtCommit(
	ctx context.Context,
	ns domain.Namespace,
	ref string,
	commit string,
) (*domain.Arrow, []byte, string, error) {
	arrow, raw, filename, err := m.ResolveArrow(ctx, ns.WithRef(commit))
	fallBack := ref != "" && ref != commit && !errors.Is(err, ErrInvalidManifest)
	if err != nil && fallBack {
		arrow, raw, filename, err = m.ResolveArrow(ctx, ns.WithRef(ref))
	}
	if err != nil {
		return nil, nil, "", fmt.Errorf("manifold: resolve arrow %s at %s (commit %s): %w", ns, ref, commit, err)
	}
	if DraftedElsewhere(arrow, ref) {
		return nil, nil, "", fmt.Errorf("manifold: resolve arrow %s at %s (commit %s): %w", ns, ref, commit, NotARelease(arrow))
	}

	arrow.Namespace = ns
	return arrow, raw, filename, nil
}

// NotARelease is the error for a draft DraftedElsewhere reports: to every
// caller, the ref holds no manifest.
func NotARelease(
	arrow *domain.Arrow,
) error {
	return fmt.Errorf("drafted from %s: %w: %w",
		arrow.Namespace.Ref(), resolver.ErrManifestNotFound, fletcher.NotFletchableError{Reason: fletcher.ReasonNotARelease})
}

// DraftedElsewhere reports whether arrow is a draft Fletcher built from a
// release other than the one at ref — what it does for a branch of a
// repository with no manifest, which publishes no release of its own. Such
// a draft is not what ref holds, so no row following ref is built from it.
func DraftedElsewhere(
	arrow *domain.Arrow,
	ref string,
) bool {
	if arrow.Namespace == "" || ref == "" || arrow.Origin() != domain.ArrowOriginInferred {
		return false
	}
	return arrow.Namespace.Ref() != ref && !strings.EqualFold(arrow.Namespace.Ref(), ref)
}

func (m *manifold) ResolveCollection(
	ctx context.Context,
	namespace domain.Namespace,
) (*domain.Collection, error) {
	data, err := m.rsv.ResolveCollection(ctx, namespace)
	if err != nil {
		return nil, err
	}
	return m.ParseCollection(data, namespace)
}

func (m *manifold) ParseCollection(
	data []byte,
	ns domain.Namespace,
) (*domain.Collection, error) {
	mod, err := m.trs.Collection(data)
	if err != nil {
		return nil, err
	}

	if err := m.rls.ValidateCollectionEntries(mod.Entries); err != nil {
		return nil, err
	}

	arrows, err := deriveArrows(mod.Entries, ns)
	if err != nil {
		return nil, err
	}

	coll := mod.Manifest
	coll.Namespace = ns
	coll.Arrows = arrows

	if err := m.rls.ValidateCollection(&coll); err != nil {
		return nil, err
	}
	return &coll, nil
}

func deriveArrows(
	entries []domain.CollectionArrowEntry,
	collNS domain.Namespace,
) ([]domain.CollectionArrow, error) {
	bare := collNS.BareNamespace()
	ref := collNS.Ref()
	arrows := make([]domain.CollectionArrow, 0, len(entries))
	for _, e := range entries {
		arrow, err := deriveArrow(e, bare, ref)
		if err != nil {
			return nil, err
		}
		arrows = append(arrows, arrow)
	}
	return arrows, nil
}

// deriveArrow settles a local member's namespace and on-disk location. The
// member lives inside the collection's own repository, at the collection's
// own commit, so its ref is the collection's — there is no other revision it
// could be at. Its identity (AUID) is the entry's explicit auid if given,
// else the last segment of its path — the path itself, in full, is kept as
// SourcePath so the file can live anywhere in the repository, not only at
// its root.
func deriveArrow(
	e domain.CollectionArrowEntry,
	bare domain.Namespace,
	ref string,
) (domain.CollectionArrow, error) {
	if e.Namespace != "" {
		return domain.CollectionArrow{Namespace: domain.Namespace(e.Namespace), IsLocal: false}, nil
	}

	// A ref authored on the path is the collection's ref restated; strip it
	// the same way Namespace itself would, rather than believe it.
	sourcePath := domain.Namespace(strings.Trim(e.Path, "/")).BareNamespace().String()

	auid := e.AUID
	if auid == "" {
		segments := strings.Split(sourcePath, "/")
		auid = segments[len(segments)-1]
	}
	if auid == "" {
		return domain.CollectionArrow{}, fmt.Errorf("manifold: arrow path %q produces an empty namespace segment", e.Path)
	}

	local := domain.Namespace(string(bare) + "/" + auid)
	return domain.CollectionArrow{
		Namespace:  local.WithRef(ref),
		IsLocal:    true,
		SourcePath: sourcePath,
	}, nil
}
