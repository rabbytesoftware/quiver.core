// Package netbridge implements dynamic port allocation and router forwarding.
package netbridge

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

// Re-export domain types so callers only need to import this package.
type Protocol = ports.Protocol
type PortAllocation = ports.PortAllocation

const (
	ProtocolTCP    = ports.ProtocolTCP
	ProtocolUDP    = ports.ProtocolUDP
	ProtocolTCPUDP = ports.ProtocolTCPUDP
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
		protocol Protocol,
		preferred int,
	) (int, error)

	// DeallocateByOwner releases all ports allocated to the given owner key.
	// Reverses router forwarding for each port before releasing.
	DeallocateByOwner(
		ctx context.Context,
		ownerKey string,
	) error
}
