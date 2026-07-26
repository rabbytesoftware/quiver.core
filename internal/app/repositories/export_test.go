package repositories

import (
	"context"

	"github.com/char2cs/asynx"

	repoarrow "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
)

// ResolveManifestFromTestable exposes resolveManifestFrom for unit tests.
func ResolveManifestFromTestable(
	axArrow asynx.Asynx[domain.Arrow],
	m manifold.Manifold,
) func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error) {
	return resolveManifestFrom(axArrow, m)
}

// IsNotFoundTestable exposes isNotFound for unit tests.
func IsNotFoundTestable(err error) bool {
	return isNotFound(err)
}

// CatalogHas exposes catalogHas for unit tests.
func CatalogHas(
	cat repoarrow.Arrow,
) discovery.KnownFn {
	return catalogHas(cat)
}
