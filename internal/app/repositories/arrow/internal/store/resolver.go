package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	manifoldresolver "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
)

func newResolver(
	v vault.Vault,
	m manifold.Manifold,
) ResolveFunc {
	return func(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error) {
		return resolve(ctx, ns, v, m)
	}
}

func resolve(
	ctx context.Context,
	ns domain.Namespace,
	v vault.Vault,
	m manifold.Manifold,
) (*domain.Arrow, error) {
	if v == nil && m == nil {
		return nil, fmt.Errorf("resolver: no vault or manifold configured")
	}
	if v == nil {
		return fetchFromManifold(ctx, ns, m)
	}
	return resolveWithVault(ctx, ns, v, m)
}

func resolveWithVault(
	ctx context.Context,
	ns domain.Namespace,
	v vault.Vault,
	m manifold.Manifold,
) (*domain.Arrow, error) {
	file, err := v.GetArrow(ctx, ns)
	if err == nil {
		return parseManifest(m, file.Content, "cached")
	}
	if errors.Is(err, vault.ErrConfirmedAbsent) {
		return nil, wrapManifoldErr("fetch from manifold (cached not-found)", manifoldresolver.ErrNotFound)
	}
	if errors.Is(err, vault.ErrStale) {
		return resolveStale(ctx, ns, v, m, file.Content)
	}
	if errors.Is(err, vault.ErrNotCached) {
		return fetchAndCache(ctx, ns, v, m)
	}
	return nil, fmt.Errorf("resolver: vault lookup: %w", err)
}

func resolveStale(
	ctx context.Context,
	ns domain.Namespace,
	v vault.Vault,
	m manifold.Manifold,
	staleContent []byte,
) (*domain.Arrow, error) {
	if m == nil {
		return nil, fmt.Errorf("resolver: stale and no manifold configured")
	}

	fresh, rawBytes, filename, err := m.ResolveArrow(ctx, ns)
	if err != nil {
		return parseManifest(m, staleContent, "stale")
	}

	if putErr := v.PutArrow(ctx, ns, Cacheable(fresh, rawBytes, filename)); putErr != nil {
		return nil, fmt.Errorf("resolver: store refreshed manifest: %w", putErr)
	}
	return fresh, nil
}

func fetchAndCache(
	ctx context.Context,
	ns domain.Namespace,
	v vault.Vault,
	m manifold.Manifold,
) (*domain.Arrow, error) {
	if m == nil {
		return nil, fmt.Errorf("resolver: not cached and no manifold configured")
	}

	fresh, rawBytes, filename, err := m.ResolveArrow(ctx, ns)
	if err != nil {
		cacheConfirmedAbsent(ctx, ns, v, err)
		return nil, wrapManifoldErr("fetch from manifold", err)
	}

	if putErr := v.PutArrow(ctx, ns, Cacheable(fresh, rawBytes, filename)); putErr != nil {
		return nil, fmt.Errorf("resolver: store manifest: %w", putErr)
	}
	return fresh, nil
}

// cacheConfirmedAbsent records a negative result in the vault so a
// namespace with no manifest at all — an old tag cut before the manifest
// convention existed, say — is not re-cloned and re-fetched on every
// single request. Scoped deliberately narrow: only a genuine
// manifoldresolver.ErrNotFound (the fetch reached the repository and
// inspected its tree) counts as confirmed absent. Any other failure
// (network blip, auth failure, timeout) must never be cached this way —
// doing so would silently convert a transient outage into a false
// "not found" for a full TTL. A failure to write the marker itself only
// costs the next request the same live re-check, so it is logged rather
// than propagated: it must never turn a definitive not-found answer into a
// different kind of error.
func cacheConfirmedAbsent(
	ctx context.Context,
	ns domain.Namespace,
	v vault.Vault,
	fetchErr error,
) {
	if !errors.Is(fetchErr, manifoldresolver.ErrNotFound) {
		return
	}
	if err := v.PutArrowNotFound(ctx, ns); err != nil {
		slog.WarnContext(ctx, "resolver: cache confirmed-absent result", "ns", ns, "err", err)
	}
}

// Cacheable pairs the raw manifest with the searchable metadata the vault
// cannot derive itself. Stars, Source and Branch stay zero — those belong to
// discovery, not to manifest resolution.
//
// Every path that caches a manifest goes through it, because a manifest cached
// without this metadata is invisible to the vault lane of search.
func Cacheable(
	arrow *domain.Arrow,
	rawBytes []byte,
	filename string,
) vault.ManifestFile {
	oses := make([]domain.OS, 0, len(arrow.Targets))
	for os := range arrow.Targets {
		oses = append(oses, os)
	}
	slices.Sort(oses)

	return vault.ManifestFile{
		Content:  rawBytes,
		Filename: filename,
		Meta: &vault.IndexMeta{
			Arrow: arrow.ArrowMeta,
			OS:    oses,
		},
	}
}

func fetchFromManifold(
	ctx context.Context,
	ns domain.Namespace,
	m manifold.Manifold,
) (*domain.Arrow, error) {
	fresh, _, _, err := m.ResolveArrow(ctx, ns)
	if err != nil {
		return nil, wrapManifoldErr("fetch from manifold", err)
	}
	return fresh, nil
}

func parseManifest(
	m manifold.Manifold,
	content []byte,
	label string,
) (*domain.Arrow, error) {
	parsed, err := m.ParseArrow(content)
	if err != nil {
		return nil, wrapManifoldErr(fmt.Sprintf("parse %s arrow", label), err)
	}
	return parsed, nil
}

// wrapManifoldErr promotes a resolver/manifold-layer sentinel to its
// app-layer equivalent so apierr.StatusAndMessage maps a real remote failure
// or invalid manifest to 404/502/422 instead of falling through to a generic
// 500. The original error stays wrapped underneath for logging.
func wrapManifoldErr(op string, err error) error {
	switch {
	case errors.Is(err, manifoldresolver.ErrNotFound):
		return fmt.Errorf("resolver: %s: %w: %w", op, apperrors.ErrNotFound, err)
	case errors.Is(err, manifoldresolver.ErrFetchFailed):
		return fmt.Errorf("resolver: %s: %w: %w", op, apperrors.ErrFetchFailed, err)
	case errors.Is(err, manifold.ErrInvalidManifest):
		return fmt.Errorf("resolver: %s: %w: %w", op, apperrors.ErrInvalidManifest, err)
	case errors.Is(err, manifold.ErrArrowNotInCollection):
		return fmt.Errorf("resolver: %s: %w: %w", op, apperrors.ErrNotFound, err)
	default:
		return fmt.Errorf("resolver: %s: %w", op, err)
	}
}
