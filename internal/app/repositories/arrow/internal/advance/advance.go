package advance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	arrowcmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/commands"
	arrowstore "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

// Advancer moves catalog rows: it records what is ahead of a row, stages a
// target's manifest, advances a row in place and adopts installed state.
type Advancer interface {
	// Advance moves ns's row in place to target, refreshing its cached
	// manifest to the one at target's commit.
	Advance(
		ctx context.Context,
		ns domain.Namespace,
		target domain.Available,
	) error
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
	// AdoptInstalled registers resolvedRef, settled against the live remote,
	// as what ns's identity already has installed, and adopts it.
	AdoptInstalled(
		ctx context.Context,
		ns domain.Namespace,
		resolvedRef string,
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
	RecordAvailable(
		ctx context.Context,
		ns domain.Namespace,
		judge Judge,
	) (*domain.Available, bool, error)
}

type advancer struct {
	store    arrowstore.Store
	axArrow  asynx.Asynx[domain.Arrow]
	vault    vault.Vault
	manifold manifold.Manifold
	// syncBadge pushes the row's Available onto the runtime badge.
	syncBadge func(ctx context.Context, ns domain.Namespace)
}

func New(
	store arrowstore.Store,
	axArrow asynx.Asynx[domain.Arrow],
	v vault.Vault,
	m manifold.Manifold,
	syncBadge func(ctx context.Context, ns domain.Namespace),
) Advancer {
	return &advancer{
		store:     store,
		axArrow:   axArrow,
		vault:     v,
		manifold:  m,
		syncBadge: syncBadge,
	}
}

func (a *advancer) identityExists(
	ctx context.Context,
	identity domain.Namespace,
) (bool, error) {
	return a.axArrow.Exists(ctx, identity.String())
}

// Advance fetches the manifest at target's commit, replaces the cached one —
// the cache is keyed by the row's identity, which a rolling selector keeps
// across commits — and moves the row to target. A git-only advance has no
// asset checksum, so its fingerprint is the commit.
func (a *advancer) Advance(
	ctx context.Context,
	ns domain.Namespace,
	target domain.Available,
) error {
	if target.Commit == "" {
		return fmt.Errorf("advance %s: target has no commit: %w", ns, apperrors.ErrInvalidNamespace)
	}

	exists, err := a.axArrow.Exists(ctx, ns.String())
	if err != nil {
		return fmt.Errorf("advance %s: %w", ns, err)
	}
	if !exists {
		return fmt.Errorf("advance %s: %w", ns, apperrors.ErrNotFound)
	}

	m, raw, filename, err := a.manifold.ResolveArrowAtCommit(ctx, ns, target.Ref, target.Commit)
	if err != nil {
		return fmt.Errorf("advance %s: %w", ns, MapResolveErr(err))
	}

	if err := a.replaceCachedManifest(ctx, ns, arrowstore.Cacheable(m, raw, filename)); err != nil {
		return fmt.Errorf("advance %s: %w", ns, err)
	}

	return a.sendAdvance(ctx, ns, m, domain.Resolved{
		Ref:         target.Ref,
		Commit:      target.Commit,
		Fingerprint: target.Commit,
	}, true)
}

// Adopt parses manifest locally and records it, with resolved, as what ns
// has installed: a new user-installed row when ns is absent, an advance when
// resolved moved, a manifest refresh when only the manifest changed, and
// nothing at all when the row already holds both — Adopt runs on every core
// boot. The cache is replaced, under filename, exactly when the row is
// written, so the two never disagree. Whoever adopts a row installed it, so
// an existing row without the user-installed flag gets it back.
func (a *advancer) Adopt(
	ctx context.Context,
	ns domain.Namespace,
	kind domain.SelectorKind,
	resolved domain.Resolved,
	manifest []byte,
	filename string,
) error {
	if ns.Validate() != nil || ns.Ref() == "" || manifold.HasEmptyComponent(ns.Ref()) {
		return fmt.Errorf("adopt %s: %w", ns, apperrors.ErrInvalidNamespace)
	}
	if filename == "" {
		return fmt.Errorf("adopt %s: manifest has no filename: %w", ns, apperrors.ErrInvalidManifest)
	}

	m, err := a.manifold.ParseArrow(manifest)
	if err != nil {
		return fmt.Errorf("adopt %s: %w: %w", ns, apperrors.ErrInvalidManifest, err)
	}
	cache := arrowstore.Cacheable(m, manifest, filename)

	exists, err := a.axArrow.Exists(ctx, ns.String())
	if err != nil {
		return fmt.Errorf("adopt %s: %w", ns, err)
	}
	if !exists {
		if err := a.replaceCachedManifest(ctx, ns, cache); err != nil {
			return fmt.Errorf("adopt %s: %w", ns, err)
		}
		return a.sendAdopted(ctx, ns, m, kind, resolved)
	}

	current, err := a.axArrow.Get(ctx, ns.String())
	if err != nil {
		return fmt.Errorf("adopt %s: %w", ns, MapGetErr(err))
	}
	if err := a.adoptOnto(ctx, ns, current, m, resolved, cache); err != nil {
		return err
	}
	if current.UserInstalled {
		return nil
	}
	_, err = a.axArrow.SendWait(ctx, arrowcmds.SetUserInstalled{Namespace: ns})
	return mapSendErr("adopt", ns, err)
}

// AdoptInstalled leaves the runtime alone, as the core's own adoption does:
// whether anything is installed stays the runtime's to report.
func (a *advancer) AdoptInstalled(
	ctx context.Context,
	ns domain.Namespace,
	resolvedRef string,
) error {
	adoption, err := a.store.ResolveAdoption(ctx, ns, resolvedRef)
	if err != nil {
		return fmt.Errorf("adopt installed: %w", MapResolveErr(err))
	}
	return a.Adopt(ctx, adoption.Identity, adoption.Kind, adoption.Resolved, adoption.Manifest, adoption.Filename)
}

// adoptOnto writes m and resolved onto the existing row current, replacing
// the cache first, or writes nothing when the row already holds both. A
// resolved with no commit naming the row's own ref is no advance: the row
// keeps the commit it learned.
func (a *advancer) adoptOnto(
	ctx context.Context,
	ns domain.Namespace,
	current domain.Arrow,
	m *domain.Arrow,
	resolved domain.Resolved,
	cache vault.ManifestFile,
) error {
	if resolved.Commit == "" && resolved.Ref == current.Resolved.Ref {
		resolved = current.Resolved
	}
	advance := current.Resolved != resolved
	if !advance && sameManifest(&current, m) {
		return nil
	}
	if err := a.replaceCachedManifest(ctx, ns, cache); err != nil {
		return fmt.Errorf("adopt %s: %w", ns, err)
	}
	if advance {
		return a.sendAdvance(ctx, ns, m, resolved, false)
	}
	err := a.sendRetryingConflicts(ctx, arrowcmds.RefreshManifest{
		Namespace: ns,
		ArrowMeta: m.ArrowMeta,
		Variables: m.Variables,
		Netbridge: m.Netbridge,
		Targets:   m.Targets,
		Readme:    m.Readme,
	})
	return mapSendErr("adopt", ns, err)
}

// sameManifest compares the manifest fields a row stores through their JSON
// form, the form the row itself is persisted in, so a field the encoding
// drops can never make an unchanged manifest look different.
func sameManifest(
	row *domain.Arrow,
	parsed *domain.Arrow,
) bool {
	a, errA := json.Marshal(manifestOf(row))
	b, errB := json.Marshal(manifestOf(parsed))
	return errA == nil && errB == nil && bytes.Equal(a, b)
}

func manifestOf(
	a *domain.Arrow,
) domain.Arrow {
	return domain.Arrow{
		ArrowMeta: a.ArrowMeta,
		Variables: a.Variables,
		Netbridge: a.Netbridge,
		Targets:   a.Targets,
		Readme:    a.Readme,
	}
}

// replaceCachedManifest deletes before writing because PutArrow only
// overwrites a file of the same name: a manifest cached as ARROW.md would
// otherwise survive next to a replacement named arrow.yaml.
func (a *advancer) replaceCachedManifest(
	ctx context.Context,
	ns domain.Namespace,
	file vault.ManifestFile,
) error {
	if err := a.vault.DeleteArrow(ctx, ns); err != nil {
		return fmt.Errorf("purge cached manifest: %w", err)
	}
	if err := a.vault.PutArrow(ctx, ns, file); err != nil {
		return arrowstore.CacheError(err)
	}
	return nil
}

// sendAdvance waits for the projections: an advance can change the
// dependency edges, and the caller reads them back. keepNewer keeps an
// Available that names something other than resolved, for an advance onto a
// target a check judged; an adoption declares state nobody judged, and clears
// it for the next check to judge.
func (a *advancer) sendAdvance(
	ctx context.Context,
	ns domain.Namespace,
	m *domain.Arrow,
	resolved domain.Resolved,
	keepNewer bool,
) error {
	err := a.sendRetryingConflicts(ctx, arrowcmds.AdvanceArrow{
		Namespace:          ns,
		ArrowMeta:          m.ArrowMeta,
		Variables:          m.Variables,
		Netbridge:          m.Netbridge,
		Targets:            m.Targets,
		Readme:             m.Readme,
		Resolved:           resolved,
		KeepNewerAvailable: keepNewer,
	})
	return mapSendErr("advance", ns, err)
}

// sendRetryingConflicts resends a write whose event depends only on the row
// existing while it fails with ErrPipelineFailed. The event is idempotent, so
// resending one that did commit is harmless, and giving up would leave the
// vault cache the caller already swapped disagreeing with the row.
func (a *advancer) sendRetryingConflicts(
	ctx context.Context,
	cmd asynxModels.Command[domain.Arrow],
) error {
	for attempt := 1; ; attempt++ {
		_, err := a.axArrow.SendWait(ctx, cmd)
		if err == nil || !errors.Is(err, asynxModels.ErrPipelineFailed) || attempt == maxWriteAttempts {
			return err
		}
	}
}

func (a *advancer) sendAdopted(
	ctx context.Context,
	ns domain.Namespace,
	m *domain.Arrow,
	kind domain.SelectorKind,
	resolved domain.Resolved,
) error {
	_, err := a.axArrow.SendWait(ctx, arrowcmds.AddArrow{
		Namespace:     ns,
		ArrowMeta:     m.ArrowMeta,
		Variables:     m.Variables,
		Netbridge:     m.Netbridge,
		Targets:       m.Targets,
		Readme:        m.Readme,
		DirectInstall: true,
		SelectorKind:  kind,
		Resolved:      resolved,
	})
	if errors.Is(err, asynxModels.ErrValidation) || errors.Is(err, asynxModels.ErrPipelineFailed) {
		return fmt.Errorf("adopt %s: %w", ns, apperrors.ErrAlreadyExists)
	}
	return mapSendErr("adopt", ns, err)
}

func mapSendErr(
	op string,
	ns domain.Namespace,
	err error,
) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, asynxModels.ErrValidation) || errors.Is(err, asynxModels.ErrPipelineFailed) {
		return fmt.Errorf("%s %s: %w", op, ns, apperrors.ErrStateViolation)
	}
	return fmt.Errorf("%s %s: %w", op, ns, err)
}
