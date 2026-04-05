package store

import (
	adapterstore "github.com/rabbytesoftware/quiver/internal/adapter/store"
	"github.com/rabbytesoftware/quiver/internal/engine/netbridge/internal/ports"
)

// PortStore is the netbridge read-model.
// Extends Store with port and owner-based queries.
type PortStore interface {
	adapterstore.Store[ports.PortAllocation, int]
	FindByPort(port int) (*ports.PortAllocation, error)
	FindByOwner(ownerKey string) ([]ports.PortAllocation, error)
}
