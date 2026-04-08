package arrow

import (
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog"
	arrowstore "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog/store"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/execution"
	"github.com/rabbytesoftware/quiver/internal/core/metadata"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine"
)

type Builder struct {
	engines           *engine.Container
	eventStore        asynxModels.Store
	runtimeEventStore asynxModels.Store
	catalog           catalog.Catalog
	catalogStore      arrowstore.ArrowCatalog
	os                domain.OS
}

func NewArrowBuilder() *Builder {
	return &Builder{}
}

func (b *Builder) WithEngines(e *engine.Container) *Builder {
	b.engines = e
	return b
}

func (b *Builder) WithEventStore(es asynxModels.Store) *Builder {
	b.eventStore = es
	return b
}

func (b *Builder) WithRuntimeEventStore(es asynxModels.Store) *Builder {
	b.runtimeEventStore = es
	return b
}

func (b *Builder) WithCatalog(c catalog.Catalog) *Builder {
	b.catalog = c
	return b
}

// WithCatalogStore injects a backing store for the catalog. The builder will
// construct a catalog.Catalog from it using the same asynx instances as the
// service. Useful in tests that need asynx-level synchronisation.
func (b *Builder) WithCatalogStore(s arrowstore.ArrowCatalog) *Builder {
	b.catalogStore = s
	return b
}

func (b *Builder) WithOS(os domain.OS) *Builder {
	b.os = os
	return b
}

// Build constructs and returns an ArrowService.
func (b *Builder) Build() (ArrowService, error) {
	if b.eventStore == nil {
		return nil, fmt.Errorf("arrow builder: event store is required")
	}

	axArrow, err := newAsynxArrow(b.eventStore)
	if err != nil {
		return nil, err
	}

	runtimeES := b.runtimeEventStore
	if runtimeES == nil {
		runtimeES = b.eventStore
	}

	axRuntime, err := newAsynxRuntime(runtimeES)
	if err != nil {
		return nil, err
	}

	var e engine.Container
	if b.engines != nil {
		e = *b.engines
	}

	cat := b.catalog
	if cat == nil {
		store := b.catalogStore
		if store == nil {
			var storeErr error
			store, storeErr = arrowstore.NewArrowCatalog(metadata.GetQuiverHome() + "/arrows.db")
			if storeErr != nil {
				return nil, storeErr
			}
		}
		cat, err = catalog.New(axArrow, axRuntime, store, e.Vault, e.Manifold)
		if err != nil {
			return nil, err
		}
	}

	exc, err := execution.New(axArrow, axRuntime, e, b.os, cat)
	if err != nil {
		return nil, err
	}

	return &arrowService{
		catalog:      cat,
		execution:    exc,
		asynxArrow:   axArrow,
		asynxRuntime: axRuntime,
		vault:        e.Vault,
	}, nil
}

func newAsynxArrow(
	es asynxModels.Store,
) (asynx.Asynx[domain.Arrow], error) {
	if es == nil {
		return nil, fmt.Errorf("asynx arrow: event store is required")
	}

	ax, err := asynx.New[domain.Arrow]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()

	return ax, err
}

func newAsynxRuntime(
	es asynxModels.Store,
) (asynx.Asynx[domainRuntime.ArrowRuntime], error) {
	if es == nil {
		return nil, fmt.Errorf("asynx runtime: event store is required")
	}

	ax, err := asynx.New[domainRuntime.ArrowRuntime]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()

	return ax, err
}
