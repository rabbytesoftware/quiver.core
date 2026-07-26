// Package netbridge implements dynamic port allocation and router forwarding.
package netbridge

import (
	"context"
	"sync"

	"github.com/char2cs/asynx"

	"github.com/rabbytesoftware/quiver.core/internal/domain/netbridge"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/ports"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/projections"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/strategies"
)

// Netbridge manages dynamic port allocation and best-effort router forwarding.
type Netbridge interface {
	// Allocate finds an available port matching the given protocol and preferred
	// port, forwards it through the router (best-effort), and returns the
	// assigned port number.
	//
	// If the preferred port is unavailable, Netbridge finds the next available
	// port in the ephemeral range (49152–65535). If forwarding fails, the port
	// is still allocated and returned — forwarding failure is non-fatal.
	//
	// Returns ErrNoPortAvailable if no port can be found.
	// Returns ErrPortOutOfRange if the preferred port is outside 1–65535.
	Allocate(
		ctx context.Context,
		ownerKey string,
		protocol netbridge.Protocol,
		preferred int,
	) (int, error)

	// DeallocateByOwner releases all ports allocated to the given owner key.
	// Reverses router forwarding for each port before releasing.
	DeallocateByOwner(
		ctx context.Context,
		ownerKey string,
	) error

	// Shutdown drains the port-allocation aggregate, blocking until every
	// in-flight command has been persisted or ctx expires. Router forwarding
	// is left in place: it is owned by the router, not by this process.
	Shutdown(
		ctx context.Context,
	) error
}

type netbridgeService struct {
	ax        asynx.Asynx[ports.PortAllocation]
	readModel store.PortStore
	stratsCh  <-chan []strategies.Strategy
	strats    []strategies.Strategy
	once      sync.Once
	portStart int
	portEnd   int
}

// New constructs a Netbridge from already-created dependencies.
// Wires the read model projection and returns the Netbridge interface.
func newNetbridge(
	ax asynx.Asynx[ports.PortAllocation],
	readModel store.PortStore,
	stratsCh <-chan []strategies.Strategy,
	portStart int,
	portEnd int,
) (Netbridge, error) {
	_, err := ax.Subscribe("port.*", projections.HandlePortEvent(readModel))
	if err != nil {
		return nil, err
	}

	return &netbridgeService{
		ax:        ax,
		readModel: readModel,
		stratsCh:  stratsCh,
		portStart: portStart,
		portEnd:   portEnd,
	}, nil
}

func (n *netbridgeService) resolveStrategies() []strategies.Strategy {
	n.once.Do(func() {
		n.strats = <-n.stratsCh
	})
	return n.strats
}

// Allocate finds an available port, attempts router forwarding, and records the allocation.
func (n *netbridgeService) Allocate(
	ctx context.Context,
	ownerKey string,
	protocol netbridge.Protocol,
	preferred int,
) (int, error) {
	port, err := findAvailablePort(ctx, preferred, n.portStart, n.portEnd, protocol, n.readModel)
	if err != nil {
		return 0, err
	}

	forwarded := false
	for _, s := range n.resolveStrategies() {
		if err := s.Forward(ctx, port, protocol); err == nil {
			forwarded = true
			break
		}
	}

	_, err = n.ax.Send(ctx, commands.AllocatePort{
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
func (n *netbridgeService) DeallocateByOwner(
	ctx context.Context,
	ownerKey string,
) error {
	allocations, err := n.readModel.FindByOwner(ctx, ownerKey)
	if err != nil {
		return err
	}

	for _, alloc := range allocations {
		for _, s := range n.resolveStrategies() {
			_ = s.Reverse(ctx, alloc.Port, alloc.Protocol)
		}

		_, sendErr := n.ax.Send(ctx, commands.DeallocatePort{Port: alloc.Port})
		if sendErr != nil {
			return sendErr
		}
	}

	return nil
}

// Shutdown drains the port-allocation aggregate.
func (n *netbridgeService) Shutdown(
	ctx context.Context,
) error {
	return n.ax.Shutdown(ctx)
}

func (n *netbridgeService) waitForProjection() {
	n.ax.WaitPublish()
}
