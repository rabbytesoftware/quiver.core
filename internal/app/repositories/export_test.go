package repositories

import (
	"context"

	"github.com/char2cs/asynx"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	repoarrow "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/collection"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/graph"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/runtime"
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

// WireCallbacks exposes wireCallbacks for unit tests.
func (c *Container) WireCallbacks() error {
	return c.wireCallbacks()
}

// ArrowGetter exposes arrowGetter for unit tests.
func ArrowGetter(
	axArrow asynx.Asynx[domain.Arrow],
) func(ctx context.Context, ns domain.Namespace) (*domain.Arrow, error) {
	return arrowGetter(axArrow)
}

// DependentsChecker exposes dependentsChecker for unit tests.
func DependentsChecker(
	g graph.Graph,
) runtime.HasDependentsFn {
	return dependentsChecker(g)
}

// CatalogLister exposes catalogLister for unit tests.
func CatalogLister(
	cat repoarrow.Arrow,
) func(ctx context.Context) ([]models.ArrowView, error) {
	return catalogLister(cat)
}

// DiscardCollection exposes discardCollection for unit tests.
func DiscardCollection(
	coll collection.Collection,
) {
	discardCollection(coll)
}
