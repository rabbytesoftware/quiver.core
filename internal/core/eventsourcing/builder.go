package eventsourcing

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/bus"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/contracts"
	internal "github.com/rabbytesoftware/quiver/internal/core/eventsourcing/internal"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/storage"
)

type Builder struct {
	store       contracts.EventStore
	bus         contracts.EventBus
	sqliteCtx   context.Context
	sqliteDB    string
	memoryStore bool
	memoryBus   bool
}

func New() *Builder {
	return &Builder{}
}

func (b *Builder) WithStore(store contracts.EventStore) *Builder {
	b.store = store
	return b
}

func (b *Builder) WithBus(bus contracts.EventBus) *Builder {
	b.bus = bus
	return b
}

func (b *Builder) WithSQLiteStore(ctx context.Context, dbName string) *Builder {
	b.sqliteCtx = ctx
	b.sqliteDB = dbName
	return b
}

func (b *Builder) WithMemoryStore() *Builder {
	b.memoryStore = true
	return b
}

func (b *Builder) WithMemoryBus() *Builder {
	b.memoryBus = true
	return b
}

func (b *Builder) Build() (*EventSourcing, error) {
	if b.store == nil && b.sqliteDB == "" {
		return nil, fmt.Errorf("store is required: use WithStore() or WithSQLiteStore()")
	}

	if b.bus == nil {
		return nil, fmt.Errorf("bus is required: use WithBus()")
	}

	registry := internal.NewRegistry()

	if b.sqliteDB != "" {
		store, err := storage.NewSQLiteEventStore(b.sqliteCtx, b.sqliteDB)
		if err != nil {
			return nil, fmt.Errorf("failed to create SQLite store: %w", err)
		}
		b.store = store
	}

	if b.memoryStore {
		b.store = storage.NewMemoryEventStore()
	}

	if b.memoryBus {
		b.bus = bus.NewEventBus()
	}

	if deserializable, ok := b.store.(interface {
		SetDeserializer(internal.DeserializeFunc)
	}); ok {
		deserializable.SetDeserializer(registry.Deserialize)
	}

	return &EventSourcing{
		store:    b.store,
		bus:      b.bus,
		registry: registry,
	}, nil
}
