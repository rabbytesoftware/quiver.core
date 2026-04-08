package quiver

import (
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/app/quiver/internal/catalog"
	quiverstore "github.com/rabbytesoftware/quiver/internal/app/quiver/internal/catalog/store"
	"github.com/rabbytesoftware/quiver/internal/core/metadata"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine"
)

type Builder struct {
	engines      *engine.Container
	eventStore   asynxModels.Store
	catalogStore quiverstore.QuiverCatalog
}

func NewQuiverBuilder() *Builder {
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

func (b *Builder) WithCatalogStore(s quiverstore.QuiverCatalog) *Builder {
	b.catalogStore = s
	return b
}

// Build constructs and returns a QuiverService.
func (b *Builder) Build() (QuiverService, error) {
	if b.eventStore == nil {
		return nil, fmt.Errorf("quiver builder: event store is required")
	}

	axQuiver, err := newAsynxQuiver(b.eventStore)
	if err != nil {
		return nil, err
	}

	cat := b.catalogStore
	if cat == nil {
		cat, err = quiverstore.NewQuiverCatalog(metadata.GetQuiverHome() + "/quivers.db")
		if err != nil {
			return nil, err
		}
	}

	var v engine.Container
	if b.engines != nil {
		v = *b.engines
	}

	c, err := catalog.New(axQuiver, cat, v.Vault, v.Manifold)
	if err != nil {
		return nil, err
	}

	return &quiverService{catalog: c, asynxQuiver: axQuiver}, nil
}

func newAsynxQuiver(
	es asynxModels.Store,
) (asynx.Asynx[domain.Quiver], error) {
	if es == nil {
		return nil, fmt.Errorf("asynx quiver: event store is required")
	}

	ax, err := asynx.New[domain.Quiver]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()

	return ax, err
}
