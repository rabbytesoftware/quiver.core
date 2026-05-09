package store

import (
	"context"

	adapterstore "github.com/rabbytesoftware/quiver.core/internal/adapter/store"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/ports"
)

type PortStore interface {
	adapterstore.Store[ports.PortAllocation, int]
	FindByPort(
		ctx context.Context,
		port int,
	) (*ports.PortAllocation, error)
	FindByOwner(
		ctx context.Context,
		ownerKey string,
	) ([]ports.PortAllocation, error)
}
