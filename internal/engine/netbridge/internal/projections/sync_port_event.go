package projections

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/ports"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/store"
)

type syncPortEvent struct {
	rm store.HostedPortStore
}

// SyncPortEvent is HandlePortEvent for a read model that lives in the event
// store's database. It runs in the event's transaction, so the allocation is
// readable the moment Send returns and a failed write drops the event with it.
func SyncPortEvent(
	rm store.HostedPortStore,
) func(context.Context, asynxModels.Tx, asynxModels.Event[ports.PortAllocation]) error {
	return syncPortEvent{rm: rm}.handle
}

func (s syncPortEvent) handle(
	ctx context.Context,
	tx asynxModels.Tx,
	evt asynxModels.Event[ports.PortAllocation],
) error {
	switch evt.EventName {
	case "port.Allocated":
		return s.rm.SaveTx(ctx, tx, evt.Aggregate)
	case "port.Deallocated":
		return s.rm.DeleteTx(ctx, tx, evt.PreviousAggregate.Port)
	}
	return nil
}
