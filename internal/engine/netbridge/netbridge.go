// Package netbridge implements dynamic port allocation and router forwarding.
package netbridge

import (
	"context"

	"github.com/char2cs/asynx"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/commands"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/store"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/strategies"
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
		protocol ports.Protocol,
		preferred int,
	) (int, error)

	// DeallocateByOwner releases all ports allocated to the given owner key.
	// Reverses router forwarding for each port before releasing.
	DeallocateByOwner(
		ctx context.Context,
		ownerKey string,
	) error
}

type netbridgeImpl struct {
	ax         asynx.Asynx[ports.PortAllocation]
	readModel  store.PortStore
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
