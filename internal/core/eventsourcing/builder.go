package eventsourcing

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/bus"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/enricher"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/idempotency"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/registry"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/store"
)

type Builder struct {
	eventStore       store.EventStore
	eventBus         bus.EventBus
	idempotencyStore *idempotency.Store
}

func New() *Builder {
	return &Builder{}
}

func (b *Builder) WithSQLiteStore(
	ctx context.Context, 
	name string,
) *Builder {
	eventStore, err := store.NewSQLiteStore(ctx, name)
	if err != nil {
		return b
	}
	b.eventStore = eventStore
	b.idempotencyStore = idempotency.NewStore()
	return b
}

func (b *Builder) WithMemoryBus() *Builder {
	b.eventBus = bus.NewMemoryBus()
	return b
}

func (b *Builder) Build() (*EventSourcing, error) {
	if b.eventStore == nil {
		return nil, fmt.Errorf("event store is required")
	}

	if b.eventBus == nil {
		return nil, fmt.Errorf("event bus is required")
	}

	if b.idempotencyStore == nil {
		b.idempotencyStore = idempotency.NewStore()
	}

	reg := registry.NewRegistry()
	enr := enricher.NewEnricher(b.eventStore)

	return &EventSourcing{
		store:            b.eventStore,
		bus:              b.eventBus,
		idempotencyStore: b.idempotencyStore,
		registry:         reg,
		enricher:         enr,
	}, nil
}
