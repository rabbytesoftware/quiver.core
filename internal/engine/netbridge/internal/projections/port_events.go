// Package projections contains Asynx event handlers for the Netbridge read model.
package projections

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/ports"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/store"
)

func HandlePortEvent(
	rm store.PortStore,
) func(context.Context, asynxModels.Event[ports.PortAllocation]) {
	return func(
		ctx context.Context,
		evt asynxModels.Event[ports.PortAllocation],
	) {
		switch evt.EventName {
		case "port.Allocated":
			_ = rm.Save(ctx, evt.Aggregate)
		case "port.Deallocated":
			_ = rm.Delete(ctx, evt.PreviousAggregate.Port)
		}
	}
}
