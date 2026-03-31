package netbridge

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/commands"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/store"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/strategies"
)

// Builder constructs a Netbridge instance.
type Builder struct {
	readModel store.Store
	dbPath    string
}

// NewBuilder returns a new builder for constructing a Netbridge instance.
func NewBuilder() *Builder {
	return &Builder{}
}

// WithStore injects a custom store.Store.
// Intended for tests. Takes precedence over WithDatabasePath.
func (b *Builder) WithStore(
	s store.Store,
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

// Build constructs the Netbridge instance.
func (b *Builder) Build(
	ctx context.Context,
) (Netbridge, error) {
	eventStore := store.NewEventStore()

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

	_, err = ax.Subscribe(
		"port.*",
		func(
			_ context.Context,
			evt asynxModels.Event[ports.PortAllocation],
		) {
			switch evt.EventName {
			case "port.Allocated":
				rm.Save(evt.Aggregate)
			case "port.Deallocated":
				rm.Delete(evt.PreviousAggregate.Port)
			}
		},
	)
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

func (b *Builder) resolveStore() (store.Store, error) {
	if b.readModel != nil {
		return b.readModel, nil
	}
	if b.dbPath != "" {
		return store.NewSQLite(b.dbPath)
	}
	return store.NewMemory(), nil
}

type netbridgeImpl struct {
	ax         asynx.Asynx[ports.PortAllocation]
	readModel  store.Store
	strategies []strategies.Strategy
}

// Allocate finds an available port, attempts router forwarding, and records the allocation.
func (n *netbridgeImpl) Allocate(
	ctx context.Context,
	ownerKey string,
	protocol ports.Protocol,
	preferred int,
) (int, error) {
	port, err := findAvailablePort(ctx, preferred, protocol, n.readModel)
	if err != nil {
		return 0, err
	}

	forwarded := false
	for _, s := range n.strategies {
		if err := s.Forward(ctx, port, protocol); err == nil {
			forwarded = true
			break
		}
	}

	err = n.ax.Send(ctx, commands.AllocatePort{
		Port:      port,
		Protocol:  protocol,
		OwnerKey:  ownerKey,
		Forwarded: forwarded,
	})
	if err != nil {
		return 0, err
	}

	return port, nil
}

// DeallocateByOwner releases all ports allocated to the given owner key.
func (n *netbridgeImpl) DeallocateByOwner(
	ctx context.Context,
	ownerKey string,
) error {
	allocations, err := n.readModel.FindByOwner(ownerKey)
	if err != nil {
		return err
	}

	for _, alloc := range allocations {
		for _, s := range n.strategies {
			s.Reverse(ctx, alloc.Port, alloc.Protocol)
		}

		sendErr := n.ax.Send(ctx, commands.DeallocatePort{Port: alloc.Port})
		if sendErr != nil {
			return sendErr
		}
	}

	return nil
}

func (n *netbridgeImpl) waitForProjection() {
	n.ax.WaitPublish()
}
