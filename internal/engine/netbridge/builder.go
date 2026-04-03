package netbridge

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	"github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver/internal/core/config"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/store"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/strategies"
)

// Builder constructs a Netbridge instance.
type Builder struct {
	readModel  store.PortStore
	eventStore models.Store
	strategies []strategies.Strategy
	portStart  int
	portEnd    int
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

// WithEventStorePath configures a SQLite database at path for event persistence.
// If not set, events are stored in memory (testing only).
func (b *Builder) WithEventStore(
	es models.Store,
) *Builder {
	b.eventStore = es
	return b
}

// WithStrategies injects a custom set of strategies.
// Intended for tests. If not set, strategies are discovered via Available() checks.
func (b *Builder) WithStrategies(
	s []strategies.Strategy,
) *Builder {
	b.strategies = s
	return b
}

// WithEphemeralPortRange configures the range of ports to use for automatic
// allocation. If not set, falls back to configuration defaults (49152-65535).
func (b *Builder) WithEphemeralPortRange(
	start int,
	end int,
) *Builder {
	b.portStart = start
	b.portEnd = end
	return b
}

func (b *Builder) Build(
	ctx context.Context,
) (Netbridge, error) {
	if b.eventStore == nil {
		return nil, fmt.Errorf("%w: %s", ErrBuildFailed, "missing EventStore")
	}

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

	portStart, portEnd := b.resolvePortRange()

	var active []strategies.Strategy
	if b.strategies != nil {
		// If strategies are provided (typically for tests), use them directly
		// without performing availability discovery (which involves network I/O).
		active = b.strategies
	} else {
		// Production path: discover available strategies via Available() checks.
		allStrategies := []strategies.Strategy{
			strategies.NewUPnP(),
			strategies.NewNATPMP(),
		}
		active = make([]strategies.Strategy, 0, len(allStrategies))
		for _, s := range allStrategies {
			if s.Available(ctx) {
				active = append(active, s)
			}
		}
	}

	return newNetbridge(ax, rm, active, portStart, portEnd)
}

func (b *Builder) resolveStore() (store.PortStore, error) {
	if b.readModel != nil {
		return b.readModel, nil
	}

	return store.NewPortMemory(), nil
}

func (b *Builder) resolvePortRange() (int, int) {
	if b.portStart > 0 && b.portEnd > 0 {
		return b.portStart, b.portEnd
	}

	cfg := config.GetNetbridge()
	return cfg.EphemeralPortStart, cfg.EphemeralPortEnd
}
