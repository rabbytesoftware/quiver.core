package netbridge

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	"github.com/char2cs/asynx/models"

	eventstore "github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite"
	"github.com/rabbytesoftware/quiver.core/internal/core/config"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/ports"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/projections"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/strategies"
)

// Builder constructs a Netbridge instance.
type Builder struct {
	readModel     store.PortStore
	dbPath        string
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

// WithDatabasePath keeps the port read model in a SQLite database at path, so
// it survives a restart alongside the event store it is projected from. Without
// it (or WithStore) the read model is in memory and starts empty on every
// process start while the port aggregates persist. Netbridge owns the opened
// database and closes it in Shutdown.
func (b *Builder) WithDatabasePath(
	path string,
) *Builder {
	b.dbPath = path
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

	rm, owned, err := b.resolveStore()
	if err != nil {
		return nil, err
	}

	ab := asynx.New[ports.PortAllocation]().
		WithEventStore(b.eventStore).
		WithSnapshotStore(b.snapshotStore).
		WithShardingOpts(asynx.ShardingOpts{
			Shards:     8,
			QueueDepth: 1000,
		})
	hosted, isHosted := rm.(store.HostedPortStore)
	if isHosted {
		ab = ab.WithSyncProjection("port.*", projections.SyncPortEvent(hosted))
	}
	ax, err := ab.Build()
	if err != nil {
		closeIfOwned(rm, owned)
		return nil, fmt.Errorf("%w: %w", ErrBuildFailed, err)
	}

	portStart, portEnd := b.resolvePortRange()

	nb, err := newNetbridge(ax, rm, b.discoverStrategies(ctx), portStart, portEnd, !isHosted)
	if err != nil {
		closeIfOwned(rm, owned)
		return nil, err
	}
	if svc, ok := nb.(*netbridgeService); ok {
		svc.ownsReadModel = owned
	}

	return nb, nil
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
			if probeStrategy(ctx, s) {
				active = append(active, s)
			}
		}
		ch <- active
		close(ch)
	}()

	return ch
}

// probeStrategy asks s whether the router speaks its protocol, giving up after
// strategyProbeTimeout so one silent protocol cannot hold up the next.
func probeStrategy(ctx context.Context, s strategies.Strategy) bool {
	ctx, cancel := context.WithTimeout(ctx, strategyProbeTimeout)
	defer cancel()

	return s.Available(ctx)
}

// resolveStore returns the read model and whether Build opened it (and so
// Netbridge must close it).
func (b *Builder) resolveStore() (store.PortStore, bool, error) {
	if b.readModel != nil {
		return b.readModel, false, nil
	}
	if b.dbPath == "" {
		return store.NewPortMemory(), false, nil
	}

	hosted, ok := b.eventStore.(eventstore.Hosted)
	if ok {
		return b.openHostedStore(hosted)
	}

	rm, err := store.NewPortSQLite(b.dbPath)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %w", ErrBuildFailed, err)
	}
	return rm, true, nil
}

func (b *Builder) openHostedStore(
	hosted eventstore.Hosted,
) (store.PortStore, bool, error) {
	rm, err := store.NewPortHosted(hosted.DB(), b.dbPath)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %w", ErrBuildFailed, err)
	}
	return rm, false, nil
}

func closeIfOwned(
	rm store.PortStore,
	owned bool,
) {
	if owned {
		_ = rm.Close()
	}
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
