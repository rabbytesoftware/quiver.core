package advance

import (
	"context"
	"errors"
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	arrowcmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/commands"
	arrowstore "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
)

// CheckAvailable re-resolves ns against a live snapshot, records the answer
// as the row's Available and syncs the runtime badge to it. Unlike the
// passive check it reports failures, and records nothing when it fails.
func (a *advancer) CheckAvailable(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Available, error) {
	exists, err := a.axArrow.Exists(ctx, ns.String())
	if err != nil {
		return nil, fmt.Errorf("check available %s: %w", ns, err)
	}
	if !exists {
		return nil, fmt.Errorf("check available %s: %w", ns, apperrors.ErrNotFound)
	}

	snap, err := a.manifold.FreshSnapshot(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("check available %s: %w", ns, MapResolveErr(err))
	}

	available, _, err := a.RecordAvailable(ctx, ns, func(current domain.Arrow) (*domain.Available, bool, error) {
		target, outdated, err := manifold.Drift(current.SelectorKind, ns.Ref(), current.Resolved, snap)
		if err != nil {
			return nil, false, fmt.Errorf("%w: %w", apperrors.ErrNotFound, err)
		}
		if !outdated {
			return nil, true, nil
		}
		return &target, true, nil
	})
	if err != nil {
		return nil, mapSendErr("check available", ns, err)
	}

	a.syncBadge(ctx, ns)
	return available, nil
}

// TargetUnmoved reports whether target's ref still stands at target's commit
// on the remote right now, reading the ref where the row's kind says it
// lives: an escaped branch pin reads its branch even when a same-name tag
// exists.
func (a *advancer) TargetUnmoved(
	ctx context.Context,
	ns domain.Namespace,
	target domain.Available,
) (bool, error) {
	current, err := a.axArrow.Get(ctx, ns.String())
	if err != nil {
		return false, fmt.Errorf("target unmoved %s: %w", ns, MapGetErr(err))
	}
	snap, err := a.manifold.FreshSnapshot(ctx, ns)
	if err != nil {
		return false, fmt.Errorf("target unmoved %s: %w", ns, MapResolveErr(err))
	}
	commit, ok := manifold.RefCommit(current.SelectorKind, ns.Ref(), target.Ref, snap)
	return ok && commit == target.Commit, nil
}

// RefreshToTarget stages target's manifest on ns's row, so an update runs the
// target's own update steps. Resolved and Available are left for the advance
// that commits the update.
func (a *advancer) RefreshToTarget(
	ctx context.Context,
	ns domain.Namespace,
	target domain.Available,
) (*domain.Arrow, error) {
	if target.Commit == "" {
		return nil, fmt.Errorf("refresh to target %s: target has no commit: %w", ns, apperrors.ErrInvalidNamespace)
	}

	exists, err := a.axArrow.Exists(ctx, ns.String())
	if err != nil {
		return nil, fmt.Errorf("refresh to target %s: %w", ns, err)
	}
	if !exists {
		return nil, fmt.Errorf("refresh to target %s: %w", ns, apperrors.ErrNotFound)
	}

	m, raw, filename, err := a.manifold.ResolveArrowAtCommit(ctx, ns, target.Ref, target.Commit)
	if err != nil {
		return nil, fmt.Errorf("refresh to target %s: %w", ns, MapResolveErr(err))
	}
	if err := a.replaceCachedManifest(ctx, ns, arrowstore.Cacheable(m, raw, filename)); err != nil {
		return nil, fmt.Errorf("refresh to target %s: %w", ns, err)
	}

	err = a.sendRetryingConflicts(ctx, arrowcmds.RefreshManifest{
		Namespace: ns,
		ArrowMeta: m.ArrowMeta,
		Variables: m.Variables,
		Netbridge: m.Netbridge,
		Targets:   m.Targets,
		Readme:    m.Readme,
	})
	if err != nil {
		return nil, mapSendErr("refresh to target", ns, err)
	}

	m.Namespace = ns
	return m, nil
}

// AddDependency catalogues the row a dependency declaration installs and
// returns its identity. A declaration resolves the same way an install does,
// so a bare one lands on its repository's default channel. A row that
// already exists is left exactly as it is.
func (a *advancer) AddDependency(
	ctx context.Context,
	ns domain.Namespace,
) (domain.Namespace, error) {
	identity, arrow, err := a.store.ResolveInstall(ctx, ns, arrowstore.CacheWhenAbsent(a.identityExists))
	if err != nil {
		return "", fmt.Errorf("add dependency %s: %w", ns, MapResolveErr(err))
	}

	exists, err := a.axArrow.Exists(ctx, identity.String())
	if err != nil {
		return "", fmt.Errorf("add dependency %s: %w", identity, err)
	}
	if exists {
		return identity, nil
	}

	_, err = a.axArrow.SendWait(ctx, arrowcmds.AddArrow{
		Namespace:    identity,
		ArrowMeta:    arrow.ArrowMeta,
		Variables:    arrow.Variables,
		Netbridge:    arrow.Netbridge,
		Targets:      arrow.Targets,
		Readme:       arrow.Readme,
		SelectorKind: arrow.SelectorKind,
		Resolved:     arrow.Resolved,
	})
	if err != nil && !errors.Is(err, asynxModels.ErrValidation) {
		return "", mapSendErr("add dependency", identity, err)
	}
	return identity, nil
}
