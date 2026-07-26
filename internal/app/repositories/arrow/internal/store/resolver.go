package store

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
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

	if putErr := v.PutArrow(ctx, ns, cacheable(fresh, rawBytes, filename)); putErr != nil {
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
		return nil, fmt.Errorf("resolver: fetch from manifold: %w", err)
	}

	if putErr := v.PutArrow(ctx, ns, cacheable(fresh, rawBytes, filename)); putErr != nil {
		return nil, fmt.Errorf("resolver: store manifest: %w", putErr)
	}
	return fresh, nil
}

// cacheable pairs the raw manifest with the searchable metadata the vault
// cannot derive itself. Stars, Source and Branch stay zero — those belong to
// discovery, not to manifest resolution.
func cacheable(
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
		return nil, fmt.Errorf("resolver: fetch from manifold: %w", err)
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
		return nil, fmt.Errorf("resolver: parse %s arrow: %w", label, err)
	}
	return parsed, nil
}
