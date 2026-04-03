// Package strategies defines the port-forwarding strategy interface and its implementations.
package strategies

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
)

// Strategy defines a port forwarding mechanism.
type Strategy interface {
	// Name returns a human-readable identifier for logging.
	Name() string

	// Available probes the network to determine if this strategy can be used.
	Available(
		ctx context.Context,
	) bool

	// Forward opens the given port on the router.
	Forward(
		ctx context.Context,
		port int,
		protocol netbridge.Protocol,
	) error

	// Reverse closes the given port on the router.
	Reverse(
		ctx context.Context,
		port int,
		protocol netbridge.Protocol,
	) error
}
