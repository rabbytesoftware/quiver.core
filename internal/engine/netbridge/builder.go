package netbridge

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	"github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/store"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/strategies"
)

// Builder constructs a Netbridge instance.
type Builder struct {
	readModel  store.PortStore
	dbPath     string
	eventStore models.Store
}

func New() *Builder {
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
func (b *Builder) WithEventStore(
	es models.Store,
) *Builder {
	b.eventStore = es
	return b
}

func (b *Builder) Build(
	ctx context.Context,
) (Netbridge, error) {
	ax, err := asynx.New[ports.PortAllocation]().
		WithEventStore(b.eventStore).
		Build()

	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuildFailed, err)
	}

	rm, err := b.resolveStore()
	if err != nil {
		return nil, err
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

	return newNetbridge(ax, rm, active)
}

func (b *Builder) resolveStore() (store.PortStore, error) {
	if b.readModel != nil {
		return b.readModel, nil
	}

	return store.NewPortMemory(), nil
}
