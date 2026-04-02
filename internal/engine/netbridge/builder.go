package netbridge

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/store"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/strategies"
)

// Builder constructs a Netbridge instance.
type Builder struct {
	readModel      store.PortStore
	dbPath         string
	eventStorePath string
}

// NewBuilder returns a new builder for constructing a Netbridge instance.
func NewBuilder() *Builder {
	return &Builder{}
}

// WithStore injects a custom store.PortStore.
// Intended for tests. Takes precedence over WithDatabasePath.
func (b *Builder) WithStore(
	s store.PortStore,
) *Builder {
	b.readModel = s
	return b
}

// WithDatabasePath configures a SQLite database at path for port persistence.
// If not set, an in-memory store is used.
func (b *Builder) WithDatabasePath(
	path string,
) *Builder {
	b.dbPath = path
	return b
}

// WithEventStorePath configures a SQLite database at path for event persistence.
// If not set, events are stored in memory (testing only).
func (b *Builder) WithEventStorePath(
	path string,
) *Builder {
	b.eventStorePath = path
	return b
}

// handlePortEvent returns a subscription handler that syncs asynx events to the read model.
func handlePortEvent(
	rm store.PortStore,
) func(context.Context, asynxModels.Event[ports.PortAllocation]) {
	return func(
		_ context.Context,
		evt asynxModels.Event[ports.PortAllocation],
	) {
		switch evt.EventName {
		case "port.Allocated":
			rm.Save(evt.Aggregate)
		case "port.Deallocated":
			rm.Delete(evt.PreviousAggregate.Port)
		}
	}
}

func (b *Builder) Build(
	ctx context.Context,
) (Netbridge, error) {
	eventStorePath := b.eventStorePath
	if eventStorePath == "" {
		eventStorePath = ":memory:"
	}

	eventStore, err := sqlite.NewEventStore(eventStorePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuildIncomplete, err)
	}

	ax, err := asynx.New[ports.PortAllocation]().
		WithEventStore(eventStore).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuildIncomplete, err)
	}

	rm, err := b.resolveStore()
	if err != nil {
		return nil, err
	}

	_, err = ax.Subscribe("port.*", handlePortEvent(rm))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuildIncomplete, err)
	}

	allStrategies := []strategies.Strategy{
		strategies.NewUPnP(),
		strategies.NewNATPMP(),
	}

	active := make([]strategies.Strategy, 0, len(allStrategies))
	for _, s := range allStrategies {
		if s.Available(ctx) {
			active = append(active, s)
		}
	}

	return &netbridgeImpl{
		ax:         ax,
		readModel:  rm,
		strategies: active,
	}, nil
}

func (b *Builder) resolveStore() (store.PortStore, error) {
	if b.readModel != nil {
		return b.readModel, nil
	}
	if b.dbPath != "" {
		return store.NewPortSQLite(b.dbPath)
	}
	return store.NewPortMemory(), nil
}
