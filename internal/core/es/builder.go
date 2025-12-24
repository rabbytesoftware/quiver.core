package eventsourcing

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/core/es/bus"
	"github.com/rabbytesoftware/quiver/internal/core/es/store"
)

type Builder struct {
	eventStore store.EventStore
	eventBus   bus.EventBus
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

	return &EventSourcing{
		store: b.eventStore,
		bus:   b.eventBus,
	}, nil
}
