package logged

import (
	"context"
	"log/slog"

	asynxModels "github.com/char2cs/asynx/models"

	devicestore "github.com/rabbytesoftware/quiver.core/internal/app/repositories/device/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

// Projection adapts projector to a durable handler that logs a failure before
// returning it, so the redelivery that follows leaves a trace.
func Projection(
	projector devicestore.Projector,
) asynxModels.DurableHandler[auth.Device] {
	return func(
		ctx context.Context,
		evt asynxModels.Event[auth.Device],
	) error {
		err := projector.Apply(ctx, evt.Aggregate)
		if err == nil {
			return nil
		}
		slog.ErrorContext(ctx, "device: projection failed", "device", evt.AggregateID, "err", err)
		return err
	}
}
