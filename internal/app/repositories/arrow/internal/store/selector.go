package store

import (
	"context"
	"fmt"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
)

func (r *storeService) ResolveInstall(
	ctx context.Context,
	ns domain.Namespace,
) (domain.Namespace, *domain.Arrow, error) {
	snap, err := r.manifold.Snapshot(ctx, ns)
	if err != nil {
		return ns, nil, fmt.Errorf("reader resolve install %s: %w", ns, wrapManifoldErr("snapshot", err))
	}

	identity, kind, err := classifyInstall(ns, snap)
	if err != nil {
		return ns, nil, fmt.Errorf("reader resolve install: %w", err)
	}

	target, err := manifold.Target(kind, identity.Ref(), snap)
	if err != nil {
		return identity, nil, fmt.Errorf("reader resolve install %s: %w: %w", identity, apperrors.ErrInvalidNamespace, err)
	}

	arrow, err := r.fetchAtCommit(ctx, identity, target.Commit)
	if err != nil {
		return identity, nil, fmt.Errorf("reader resolve install %s: %w", identity, err)
	}

	arrow.SelectorKind = kind
	arrow.Resolved = domain.Resolved{
		Ref:         target.Ref,
		Commit:      target.Commit,
		Fingerprint: target.Commit,
	}
	return identity, arrow, nil
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

// fetchAtCommit caches the manifest under the identity, not under the
// resolved ref, because the identity is what every later lookup of the row
// asks the vault for.
func (r *storeService) fetchAtCommit(
	ctx context.Context,
	identity domain.Namespace,
	commit string,
) (*domain.Arrow, error) {
	arrow, raw, filename, err := r.manifold.ResolveArrowAtCommit(ctx, identity, commit)
	if err != nil {
		return nil, wrapManifoldErr("fetch at commit", err)
	}
	if r.vault == nil {
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

func (r *storeService) CheckDrift(
	ctx context.Context,
	arrow domain.Arrow,
) (*domain.Available, bool) {
	snap, err := r.manifold.Snapshot(ctx, arrow.Namespace)
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
