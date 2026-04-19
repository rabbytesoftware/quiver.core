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
		entry, _, err := v.GetArrow(ctx, ns)

		if err == nil {
			return &entry.Manifest, nil
		}

		if errors.Is(err, vault.ErrStale) {
			fresh, manifoldErr := m.ResolveArrow(ctx, ns)
			if manifoldErr != nil {
				return &entry.Manifest, nil
			}

			if _, putErr := v.PutArrow(ctx, ns, fresh); putErr != nil {
				return nil, fmt.Errorf("manifest: store refreshed manifest: %w", putErr)
			}

			return fresh, nil
		}

		if errors.Is(err, vault.ErrNotCached) {
			fresh, manifoldErr := m.ResolveArrow(ctx, ns)
			if manifoldErr != nil {
				return nil, fmt.Errorf("manifest: fetch from manifold: %w", manifoldErr)
			}

			if _, putErr := v.PutArrow(ctx, ns, fresh); putErr != nil {
				return nil, fmt.Errorf("manifest: store manifest: %w", putErr)
			}

			return fresh, nil
		}

		return nil, fmt.Errorf("manifest: vault lookup: %w", err)
	}
}
