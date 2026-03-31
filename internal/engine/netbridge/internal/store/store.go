// Package store provides the read model and event store implementations for netbridge.
package store

import "github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"

// Store is the query side for allocated ports.
// Updated via Asynx subscriptions: port.Allocated → Save, port.Deallocated → Delete.
type Store interface {
	Save(
		allocation ports.PortAllocation,
	) error
	Delete(
		port int,
	) error
	FindByPort(
		port int,
	) (*ports.PortAllocation, error)
	FindByOwner(
		ownerKey string,
	) ([]ports.PortAllocation, error)
}
