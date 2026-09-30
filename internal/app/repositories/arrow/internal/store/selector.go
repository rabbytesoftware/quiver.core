package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver/resolvers"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

// ExistsFunc reports whether identity already has a catalog row.
type ExistsFunc func(ctx context.Context, identity domain.Namespace) (bool, error)

// InstallOption configures ResolveInstall.
type InstallOption func(*installOpts)

type installOpts struct {
	exists  ExistsFunc
	preview bool
}

// CacheWhenAbsent caches the resolved manifest under the identity, but only
// when exists reports no row for it yet.
func CacheWhenAbsent(
	exists ExistsFunc,
) InstallOption {
	return func(o *installOpts) {
		o.exists = exists
	}
}

// Preview serves a read-only preview from a fresh vault entry at the ref the
// selector points at, so a manifest discovery cached there is not fetched or
// drafted again, and caches nothing itself. An install never takes it: it
// reads the manifest at the exact commit it records.
func Preview() InstallOption {
	return func(o *installOpts) {
		o.preview = true
	}
}

func (r *storeService) ResolveInstall(
	ctx context.Context,
	ns domain.Namespace,
	opts ...InstallOption,
) (domain.Namespace, *domain.Arrow, error) {
	var o installOpts
	for _, opt := range opts {
		opt(&o)
	}

	identity, kind, snap, err := identify(ctx, ns, r.manifold.Snapshot)
	if err != nil {
		return identity, nil, fmt.Errorf("reader resolve install %w", err)
	}

	arrow, err := r.installAt(ctx, identity, kind, snap, o)
	if err == nil || ns.Ref() != "" || !errors.Is(err, apperrors.ErrNotFound) {
		return identity, arrow, err
	}
	if fallback, arrow, ok := r.installFallback(ctx, ns, identity.Ref(), snap, o); ok {
		return fallback, arrow, nil
	}
	return identity, nil, err
}

func (r *storeService) installAt(
	ctx context.Context,
	identity domain.Namespace,
	kind domain.SelectorKind,
	snap domain.RefSnapshot,
	o installOpts,
) (*domain.Arrow, error) {
	target, err := manifold.Target(kind, identity.Ref(), snap)
	if err != nil {
		return nil, fmt.Errorf("reader resolve install %s: %w: %w", identity, targetSentinel(kind), err)
	}

	arrow, err := r.readTarget(ctx, identity, target, o)
	if err != nil {
		return nil, fmt.Errorf("reader resolve install %s: %w", identity, err)
	}

	arrow.SelectorKind = kind
	arrow.Resolved = resolvedAt(target)
	return arrow, nil
}

// installFallback settles a refless install whose default channel serves no
// manifest — a latest release published without one, say — on the first other
// listed channel that does, and then on the HEAD branch.
func (r *storeService) installFallback(
	ctx context.Context,
	ns domain.Namespace,
	tried string,
	snap domain.RefSnapshot,
	o installOpts,
) (domain.Namespace, *domain.Arrow, bool) {
	for _, selector := range fallbackSelectors(snap, tried) {
		kind, err := manifold.ClassifySelector(selector, snap)
		if err != nil {
			continue
		}
		identity := ns.WithRef(selector)
		if arrow, err := r.installAt(ctx, identity, kind, snap, o); err == nil {
			return identity, arrow, true
		}
	}
	return ns, nil, false
}

func fallbackSelectors(
	snap domain.RefSnapshot,
	tried string,
) []string {
	selectors := []string{tried}
	for _, channel := range resolvers.NewestFirst(manifold.ChannelsOf(snap)) {
		if !channel.IsDefaultBranchFallback && !slices.Contains(selectors, channel.Name) {
			selectors = append(selectors, channel.Name)
		}
	}
	if snap.Head != "" && !slices.Contains(selectors, snap.Head) {
		selectors = append(selectors, snap.Head)
	}
	return selectors[1:]
}

type snapshotFunc func(ctx context.Context, ns domain.Namespace) (domain.RefSnapshot, error)

// identify settles the identity and selector kind ns is catalogued under,
// against the snapshot take returns.
func identify(
	ctx context.Context,
	ns domain.Namespace,
	take snapshotFunc,
) (domain.Namespace, domain.SelectorKind, domain.RefSnapshot, error) {
	snap, err := take(ctx, ns)
	if err != nil {
		return ns, domain.SelectorPin, snap, fmt.Errorf("%s: %w", ns, wrapManifoldErr("snapshot", err))
	}

	identity, kind, err := classifyInstall(ns, snap)
	if err != nil {
		return ns, kind, snap, err
	}
	return identity, kind, snap, nil
}

// resolvedAt is what a row records for target: a git-only resolution has no
// asset checksum, so its fingerprint is the commit.
func resolvedAt(
	target domain.Available,
) domain.Resolved {
	return domain.Resolved{
		Ref:         target.Ref,
		Commit:      target.Commit,
		Fingerprint: target.Commit,
	}
}

func classifyInstall(
	ns domain.Namespace,
	snap domain.RefSnapshot,
) (domain.Namespace, domain.SelectorKind, error) {
	if ns.Ref() == "" {
		channel, err := manifold.DefaultChannel(snap)
		if err != nil {
			return ns, domain.SelectorChannel, fmt.Errorf("%s: %w: %w", ns, apperrors.ErrNotFound, err)
		}
		ns = ns.WithRef(channel)
	}

	kind, err := manifold.ClassifySelector(ns.Ref(), snap)
	if err != nil {
		return ns, kind, fmt.Errorf("%s: %w: %w", ns, apperrors.ErrInvalidNamespace, err)
	}
	if kind == domain.SelectorCommit {
		return ns.WithRef(strings.ToLower(ns.Ref())), kind, nil
	}
	return ns, kind, nil
}

// targetSentinel separates a selector that names nothing from a channel that
// exists but has no commit to install.
func targetSentinel(
	kind domain.SelectorKind,
) error {
	if kind.Family() == domain.SelectorChannel {
		return apperrors.ErrNotFound
	}
	return apperrors.ErrInvalidNamespace
}

func (r *storeService) readTarget(
	ctx context.Context,
	identity domain.Namespace,
	target domain.Available,
	o installOpts,
) (*domain.Arrow, error) {
	if o.preview {
		arrow, _, _, err := r.manifestAt(ctx, identity, target)
		return arrow, err
	}
	return r.fetchAtCommit(ctx, identity, target, o.exists)
}

// manifestAt is identity's manifest at target's commit. A copy the vault holds
// for that very release — its ref at its commit — is reused, under the identity or at target's ref
// (where discovery files what it built); only without one is it read from
// the host. A tag that moved names another commit, so its stale copy is
// never served.
func (r *storeService) manifestAt(
	ctx context.Context,
	identity domain.Namespace,
	target domain.Available,
) (*domain.Arrow, []byte, string, error) {
	if arrow, file, ok := r.CachedAtCommit(ctx, identity, target); ok {
		return arrow, file.Content, file.Filename, nil
	}
	return r.fetchManifestAt(ctx, identity, target)
}

// fetchManifestAt reads identity's manifest at target's commit from the host.
// A definitive absence is recorded at target's ref, for that commit, so a
// version check stops offering a target no update could install.
func (r *storeService) fetchManifestAt(
	ctx context.Context,
	identity domain.Namespace,
	target domain.Available,
) (*domain.Arrow, []byte, string, error) {
	arrow, raw, filename, err := r.manifold.ResolveArrowAtCommit(ctx, identity, target.Ref, target.Commit)
	if err == nil {
		return arrow, raw, filename, nil
	}
	mapped := wrapManifoldErr("fetch at commit", err)
	if errors.Is(mapped, apperrors.ErrNotFound) {
		r.RecordAbsent(ctx, identity, target)
	}
	return nil, nil, "", mapped
}

func (r *storeService) CachedAtCommit(
	ctx context.Context,
	identity domain.Namespace,
	target domain.Available,
) (*domain.Arrow, vault.ManifestFile, bool) {
	if r.vault == nil || target.Commit == "" {
		return nil, vault.ManifestFile{}, false
	}
	for _, key := range []domain.Namespace{identity, identity.WithRef(target.Ref)} {
		file, err := r.vault.GetArrow(ctx, key)
		if err != nil && !errors.Is(err, vault.ErrStale) {
			continue
		}
		if file.Ref != target.Ref || file.Commit == "" || !strings.EqualFold(file.Commit, target.Commit) {
			continue
		}
		arrow, err := r.manifold.ParseArrow(file.Content)
		if err != nil {
			continue
		}
		arrow.Namespace = identity
		return arrow, file, true
	}
	return nil, vault.ManifestFile{}, false
}

// RecordAbsent marks target's ref as holding no manifest at target's commit.
// A ref that is the identity itself is left alone: the identity's entry is
// the installed row's own manifest.
func (r *storeService) RecordAbsent(
	ctx context.Context,
	identity domain.Namespace,
	target domain.Available,
) {
	key := identity.WithRef(target.Ref)
	if r.vault == nil || key == identity || target.Commit == "" {
		return
	}
	if err := r.vault.PutArrowNotFound(ctx, key, target.Commit); err != nil {
		slog.WarnContext(ctx, "store: record absent target", "ns", key, "err", err)
	}
}

// knownAbsent reports whether a fetch already found no manifest at target's
// commit: a local read, no request.
func (r *storeService) knownAbsent(
	ctx context.Context,
	identity domain.Namespace,
	target domain.Available,
) bool {
	key := identity.WithRef(target.Ref)
	if r.vault == nil || key == identity {
		return false
	}
	file, err := r.vault.GetArrow(ctx, key)
	return errors.Is(err, vault.ErrConfirmedAbsent) && file.Commit != "" && strings.EqualFold(file.Commit, target.Commit)
}

// fetchAtCommit caches the manifest under the identity, not under the
// resolved ref, because the identity is what every later lookup of the row
// asks the vault for. An installed identity keeps its cache: it belongs to
// the commit the row records, which only an update moves.
func (r *storeService) fetchAtCommit(
	ctx context.Context,
	identity domain.Namespace,
	target domain.Available,
	exists ExistsFunc,
) (*domain.Arrow, error) {
	arrow, raw, filename, err := r.manifestAt(ctx, identity, target)
	if err != nil {
		return nil, err
	}
	if exists == nil {
		return arrow, nil
	}

	installed, err := exists(ctx, identity)
	if err != nil {
		return nil, fmt.Errorf("check installed: %w", err)
	}
	if installed {
		return arrow, nil
	}

	if err := r.vault.DeleteArrow(ctx, identity); err != nil {
		return nil, fmt.Errorf("purge cached manifest: %w", err)
	}
	file := Cacheable(arrow, raw, filename)
	file.Ref = target.Ref
	file.Commit = target.Commit
	if err := r.vault.PutArrow(ctx, identity, file); err != nil {
		return nil, CacheError(err)
	}
	return arrow, nil
}

// Adoption is declared installed state, settled against the remote: the
// identity and selector kind it is catalogued under, what it has installed,
// and the manifest at that commit as the remote serves it.
type Adoption struct {
	Identity domain.Namespace
	Kind     domain.SelectorKind
	Resolved domain.Resolved
	Manifest []byte
	Filename string
}

// ResolveAdoption reads the live remote rather than the snapshot cache: the
// declared ref may have been published after the cache was filled.
func (r *storeService) ResolveAdoption(
	ctx context.Context,
	ns domain.Namespace,
	resolvedRef string,
) (Adoption, error) {
	if strings.TrimSpace(resolvedRef) == "" {
		return Adoption{}, fmt.Errorf("reader resolve adoption %s: no resolved ref: %w", ns, apperrors.ErrInvalidNamespace)
	}

	identity, kind, snap, err := identify(ctx, ns, r.manifold.FreshSnapshot)
	if err != nil {
		return Adoption{}, fmt.Errorf("reader resolve adoption %w", err)
	}

	declared, err := manifold.Admit(kind, identity.Ref(), resolvedRef, snap)
	if err != nil {
		return Adoption{}, fmt.Errorf("reader resolve adoption %s: %w: %w", identity, admitSentinel(err), err)
	}

	_, raw, filename, err := r.manifestAt(ctx, identity, declared)
	if err != nil {
		return Adoption{}, fmt.Errorf("reader resolve adoption %s: %w", identity, err)
	}

	return Adoption{
		Identity: identity,
		Kind:     kind,
		Resolved: resolvedAt(declared),
		Manifest: raw,
		Filename: filename,
	}, nil
}

// admitSentinel separates a ref the remote does not hold from one the
// selector could never resolve to.
func admitSentinel(
	err error,
) error {
	if errors.Is(err, manifold.ErrNotAdmitted) {
		return apperrors.ErrInvalidNamespace
	}
	return apperrors.ErrNotFound
}

func (r *storeService) CheckDrift(
	ctx context.Context,
	arrow domain.Arrow,
) (*domain.Available, bool) {
	snap, err := r.manifold.FreshSnapshot(ctx, arrow.Namespace)
	if err != nil {
		return nil, false
	}

	target, outdated, err := manifold.Drift(arrow.SelectorKind, arrow.Namespace.Ref(), arrow.Resolved, snap)
	if err != nil {
		return nil, false
	}
	if !outdated || r.knownAbsent(ctx, arrow.Namespace, target) {
		return nil, true
	}
	return &target, true
}
