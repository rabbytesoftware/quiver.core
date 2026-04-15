package arrow

import (
	"fmt"
	"path/filepath"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog"
	arrowstore "github.com/rabbytesoftware/quiver/internal/app/arrow/internal/catalog/store"
	"github.com/rabbytesoftware/quiver/internal/app/arrow/internal/execution"
	apphub "github.com/rabbytesoftware/quiver/internal/app/hub"
	"github.com/rabbytesoftware/quiver/internal/core/paths"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine"
)

type Builder struct {
	engines           *engine.Container
	eventStore        asynxModels.Store
	runtimeEventStore asynxModels.Store
	catalog           catalog.Catalog
	os                domain.OS
	asynxArrow        asynx.Asynx[domain.Arrow]
	asynxRuntime      asynx.Asynx[domainRuntime.ArrowRuntime]
	hub               apphub.WebSocketHub
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

func (b *Builder) WithOS(os domain.OS) *Builder {
	b.os = os
	return b
}

func (b *Builder) WithAsynxArrow(axArrow asynx.Asynx[domain.Arrow]) *Builder {
	b.asynxArrow = axArrow
	return b
}

func (b *Builder) WithAsynxRuntime(axRuntime asynx.Asynx[domainRuntime.ArrowRuntime]) *Builder {
	b.asynxRuntime = axRuntime
	return b
}

func (b *Builder) WithWebSocketHub(h apphub.WebSocketHub) *Builder {
	b.hub = h
	return b
}

// Build constructs and returns an ArrowService.
func (b *Builder) Build() (ArrowService, error) {
	if b.eventStore == nil && b.asynxArrow == nil {
		return nil, fmt.Errorf("arrow builder: event store is required")
	}

	axArrow := b.asynxArrow
	var err error
	if axArrow == nil {
		axArrow, err = newAsynxArrow(b.eventStore)
		if err != nil {
			return nil, err
		}
	}

	axRuntime := b.asynxRuntime
	if axRuntime == nil {
		runtimeES := b.runtimeEventStore
		if runtimeES == nil {
			runtimeES = b.eventStore
		}

		axRuntime, err = newAsynxRuntime(runtimeES)
		if err != nil {
			return nil, err
		}
	}

	var e engine.Container
	if b.engines != nil {
		e = *b.engines
	}

	cat := b.catalog
	if cat == nil {
		storePath, storePathErr := paths.Store()
		if storePathErr != nil {
			return nil, fmt.Errorf("arrow builder: %w", storePathErr)
		}
		store, storeErr := arrowstore.NewArrowCatalog(filepath.Join(storePath, "arrows.db"))
		if storeErr != nil {
			return nil, storeErr
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

	if b.hub != nil {
		if err := registerWSProjections(axArrow, axRuntime, b.hub); err != nil {
			return nil, fmt.Errorf("arrow builder: %w", err)
		}
	}

	return &arrowService{
		catalog:      cat,
		execution:    exc,
		asynxRuntime: axRuntime,
		vault:        e.Vault,
		manifold:     e.Manifold,
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
