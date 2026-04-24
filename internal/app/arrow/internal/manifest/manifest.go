package manifest

import (
	"context"
	"errors"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
)

// ResolveFunc resolves the canonical manifest for a namespace.
type ResolveFunc func(
	ctx context.Context,
	ns domain.Namespace,
) (*domain.Arrow, error)

// New returns a ResolveFunc that resolves the canonical manifest for a namespace
// using the vault as a cache and the manifold as the remote source.
func New(
	v vault.Vault,
	m manifold.Manifold,
) ResolveFunc {
	return func(
		ctx context.Context,
		ns domain.Namespace,
	) (*domain.Arrow, error) {
		file, err := v.GetArrow(ctx, ns)

		if err == nil {
			parsed, parseErr := m.ParseArrow(file.Content)
			if parseErr != nil {
				return nil, fmt.Errorf("manifest: parse cached arrow: %w", parseErr)
			}
			return parsed, nil
		}

		if errors.Is(err, vault.ErrStale) {
			fresh, rawBytes, filename, manifoldErr := m.ResolveArrow(ctx, ns)
			if manifoldErr != nil {
				parsed, parseErr := m.ParseArrow(file.Content)
				if parseErr != nil {
					return nil, fmt.Errorf("manifest: parse stale arrow: %w", parseErr)
				}
				return parsed, nil
			}

			if putErr := v.PutArrow(ctx, ns, vault.ManifestFile{Content: rawBytes, Filename: filename}); putErr != nil {
				return nil, fmt.Errorf("manifest: store refreshed manifest: %w", putErr)
			}

			return fresh, nil
		}

		if errors.Is(err, vault.ErrNotCached) {
			fresh, rawBytes, filename, manifoldErr := m.ResolveArrow(ctx, ns)
			if manifoldErr != nil {
				return nil, fmt.Errorf("manifest: fetch from manifold: %w", manifoldErr)
			}

			if putErr := v.PutArrow(ctx, ns, vault.ManifestFile{Content: rawBytes, Filename: filename}); putErr != nil {
				return nil, fmt.Errorf("manifest: store manifest: %w", putErr)
			}

			return fresh, nil
		}

		return nil, fmt.Errorf("manifest: vault lookup: %w", err)
	}
}
