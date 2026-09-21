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
	"github.com/rabbytesoftware/quiver.core/internal/core/selfupdate"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	wizardPkg "github.com/rabbytesoftware/quiver.core/internal/engine/wizard"
)

// CatalogHas exposes catalogHas for unit tests.
func CatalogHas(
	cat repoarrow.Arrow,
) discovery.KnownFn {
	return catalogHas(cat)
}

// WireCallbacks exposes wireCallbacks for unit tests.
func (c *Container) WireCallbacks(
	trig *selfupdate.Trigger,
) error {
	return c.wireCallbacks(trig)
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

// PreinstalledDetection exposes preinstalledDetection for unit tests.
func PreinstalledDetection(
	w wizardPkg.Wizard,
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	os domain.OS,
) []repoarrow.Option {
	return preinstalledDetection(w, axRuntime, os)
}

// PreinstalledProbe exposes preinstalledProbe for unit tests.
func PreinstalledProbe(
	w wizardPkg.Wizard,
) repoarrow.PreinstalledProbeFn {
	return preinstalledProbe(w)
}
