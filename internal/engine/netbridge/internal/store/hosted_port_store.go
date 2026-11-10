package store

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/ports"
)

// HostedPortStore is a PortStore that lives in the port aggregate's events
// database, so its writes can run inside an event's transaction.
type HostedPortStore interface {
	PortStore

	// SaveTx writes alloc through tx, committing or rolling back with the
	// event.
	SaveTx(
		ctx context.Context,
		tx asynxModels.Tx,
		alloc ports.PortAllocation,
	) error

	// DeleteTx removes the allocation of port through tx.
	DeleteTx(
		ctx context.Context,
		tx asynxModels.Tx,
		port int,
	) error
}
