package arrow

import (
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	arrowproj "github.com/rabbytesoftware/quiver/internal/app/arrow/projections"
	arrowstore "github.com/rabbytesoftware/quiver/internal/app/arrow/store"
	"github.com/rabbytesoftware/quiver/internal/core/metadata"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	"github.com/rabbytesoftware/quiver/internal/engine"
)

type Builder struct {
	engines           *engine.Container
	eventStore        asynxModels.Store
	runtimeEventStore asynxModels.Store
	catalog           arrowstore.ArrowCatalog
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

func (b *Builder) WithCatalog(c arrowstore.ArrowCatalog) *Builder {
	b.catalog = c
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

	catalog := b.catalog
	if catalog == nil {
		catalog, err = arrowstore.NewArrowCatalog(metadata.GetQuiverHome() + "/arrows.db")
		if err != nil {
			return nil, err
		}
	}

	var e engine.Container
	if b.engines != nil {
		e = *b.engines
	}

	svc := &arrowService{
		asynxArrow:   axArrow,
		asynxRuntime: axRuntime,
		catalog:      catalog,
		engines:      e,
		os:           b.os,
	}

	if err = arrowproj.Init(
		axArrow,
		axRuntime,
		b.catalog,
		e.Wizard,
	); err != nil {
		return nil, err
	}

	return svc, nil
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
