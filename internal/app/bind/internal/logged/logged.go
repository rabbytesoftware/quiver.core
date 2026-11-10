package logged

import (
	"context"
	"log/slog"

	asynxModels "github.com/char2cs/asynx/models"
)

// Handler adapts a durable handler to a plain subscription: a failure cannot
// be retried there, so it is logged instead.
func Handler[T any](
	h asynxModels.DurableHandler[T],
) asynxModels.ProjectionHandler[T] {
	return func(
		ctx context.Context,
		evt asynxModels.Event[T],
	) {
		err := h(ctx, evt)
		if err == nil {
			return
		}
		slog.ErrorContext(ctx, "bind: handler failed",
			"event", evt.EventName,
			"aggregate", evt.AggregateID,
			"err", err,
		)
	}
}
