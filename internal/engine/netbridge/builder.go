package netbridge

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	"github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/ports"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/strategies"
)

// Builder constructs a Netbridge instance.
type Builder struct {
	readModel     store.PortStore
	eventStore    models.Store
	snapshotStore models.SnapshotStore
	strategies    []strategies.Strategy
	forwarding    *bool
	portStart     int
	portEnd       int
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

// WithEventStore injects the asynx event store used for command persistence.
// Required; Build returns an error if it is not set.
func (b *Builder) WithEventStore(
	es models.Store,
) *Builder {
	b.eventStore = es
	return b
}

// WithSnapshotStore injects the asynx snapshot store used for aggregate
// snapshots. Required; Build returns an error if it is not set.
func (b *Builder) WithSnapshotStore(
	ss models.SnapshotStore,
) *Builder {
	b.snapshotStore = ss
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

// WithForwarding enables or disables best-effort router port forwarding.
// When disabled, ports are still allocated and recorded, but no UPnP or
// NAT-PMP call is ever made: strategy discovery is skipped at build time and
// every allocation is recorded as not forwarded. If not set, falls back to
// netbridge.enabled in configuration.
func (b *Builder) WithForwarding(
	enabled bool,
) *Builder {
	b.forwarding = &enabled
	return b
}

func (b *Builder) Build(
	ctx context.Context,
) (Netbridge, error) {
	if b.eventStore == nil {
		return nil, fmt.Errorf("%w: %s", ErrBuildFailed, "missing EventStore")
	}
	if b.snapshotStore == nil {
		return nil, fmt.Errorf("%w: %s", ErrBuildFailed, "missing SnapshotStore")
	}

	ax, err := asynx.New[ports.PortAllocation]().
		WithEventStore(b.eventStore).
		WithSnapshotStore(b.snapshotStore).
		WithShardingOpts(asynx.ShardingOpts{
			Shards:     8,
			QueueDepth: 1000,
		}).
		Build()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuildFailed, err)
	}

	rm, err := b.resolveStore()
	if err != nil {
		return nil, err
	}

	portStart, portEnd := b.resolvePortRange()

	return newNetbridge(ax, rm, b.discoverStrategies(ctx), portStart, portEnd)
}

func (b *Builder) discoverStrategies(
	ctx context.Context,
) <-chan []strategies.Strategy {
	ch := make(chan []strategies.Strategy, 1)

	if !b.resolveForwarding() {
		ch <- nil
		close(ch)
		return ch
	}

	if b.strategies != nil {
		ch <- b.strategies
		close(ch)
		return ch
	}

	go func() {
		all := []strategies.Strategy{
			strategies.NewUPnP(),
			strategies.NewNATPMP(),
		}
		active := make([]strategies.Strategy, 0, len(all))
		for _, s := range all {
			if s.Available(ctx) {
				active = append(active, s)
			}
		}
		ch <- active
		close(ch)
	}()

	return ch
}

func (b *Builder) resolveStore() (store.PortStore, error) {
	if b.readModel != nil {
		return b.readModel, nil
	}

	return store.NewPortMemory(), nil
}

func (b *Builder) resolveForwarding() bool {
	if b.forwarding != nil {
		return *b.forwarding
	}

	return config.GetNetbridge().Enabled
}

func (b *Builder) resolvePortRange() (int, int) {
	if b.portStart > 0 && b.portEnd > 0 {
		return b.portStart, b.portEnd
	}

	cfg := config.GetNetbridge()
	return cfg.EphemeralPortStart, cfg.EphemeralPortEnd
}
