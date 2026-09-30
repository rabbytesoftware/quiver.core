package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
)

// ExistsFunc reports whether identity already has a catalog row.
type ExistsFunc func(ctx context.Context, identity domain.Namespace) (bool, error)

// InstallOption configures ResolveInstall.
type InstallOption func(*installOpts)

type installOpts struct {
	exists ExistsFunc
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

	target, err := manifold.Target(kind, identity.Ref(), snap)
	if err != nil {
		return identity, nil, fmt.Errorf("reader resolve install %s: %w: %w", identity, targetSentinel(kind), err)
	}

	arrow, err := r.fetchAtCommit(ctx, identity, target, o.exists)
	if err != nil {
		return identity, nil, fmt.Errorf("reader resolve install %s: %w", identity, err)
	}

	arrow.SelectorKind = kind
	arrow.Resolved = resolvedAt(target)
	return identity, arrow, nil
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
		return ns.WithRef(channel), domain.SelectorChannel, nil
	}

	kind, err := manifold.ClassifySelector(ns.Ref(), snap)
	if err != nil {
		return ns, kind, fmt.Errorf("%s: %w: %w", ns, apperrors.ErrInvalidNamespace, err)
	}
	return ns, kind, nil
}

// targetSentinel separates a selector that names nothing from a channel that
// exists but has no commit to install.
func targetSentinel(
	kind domain.SelectorKind,
) error {
	if kind == domain.SelectorChannel {
		return apperrors.ErrNotFound
	}
	return apperrors.ErrInvalidNamespace
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
	arrow, raw, filename, err := r.manifold.ResolveArrowAtCommit(ctx, identity, target.Ref, target.Commit)
	if err != nil {
		return nil, wrapManifoldErr("fetch at commit", err)
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
	if err := r.vault.PutArrow(ctx, identity, Cacheable(arrow, raw, filename)); err != nil {
		return nil, fmt.Errorf("cache manifest: %w", err)
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

	_, raw, filename, err := r.manifold.ResolveArrowAtCommit(ctx, identity, declared.Ref, declared.Commit)
	if err != nil {
		return Adoption{}, fmt.Errorf("reader resolve adoption %s: %w", identity, wrapManifoldErr("fetch at commit", err))
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
	if !outdated {
		return nil, true
	}
	return &target, true
}
