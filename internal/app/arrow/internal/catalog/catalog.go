package catalog

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog/store"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

// Catalog is the read model for arrows: a pure aggregation of versioned
// arrows grouped by bare namespace. It is populated via event projections
// from the asynx event store and does not handle writes directly.
type Catalog interface {
	// Get returns the aggregated view model for a bare namespace,
	// or nil if not found. Strips @ref from ns if present.
	Get(
		ctx context.Context,
		ns domain.Namespace,
	) (*store.ArrowViewModel, error)
	// List returns all aggregated view models.
	List(
		ctx context.Context,
	) ([]store.ArrowViewModel, error)
}

type catalogService struct {
	axArrow   asynx.Asynx[domain.Arrow]
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime]
	store     store.ArrowCatalog
}

func New(
	axArrow asynx.Asynx[domain.Arrow],
	axRuntime asynx.Asynx[domainRuntime.ArrowRuntime],
	cat store.ArrowCatalog,
) (Catalog, error) {
	c := &catalogService{
		axArrow:   axArrow,
		axRuntime: axRuntime,
		store:     cat,
	}

	if err := c.registerProjections(); err != nil {
		return nil, fmt.Errorf("catalog: register projections: %w", err)
	}

	return c, nil
}

func (c *catalogService) Get(
	ctx context.Context,
	ns domain.Namespace,
) (*store.ArrowViewModel, error) {
	bareNs := ns.BareNamespace()
	return c.store.Get(ctx, bareNs)
}

func (c *catalogService) List(
	ctx context.Context,
) ([]store.ArrowViewModel, error) {
	return c.store.List(ctx)
}
