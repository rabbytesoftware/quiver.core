package bind

import (
	"context"
	"log/slog"

	asynxModels "github.com/char2cs/asynx/models"
)

type loggedHandler[T any] struct {
	handler asynxModels.DurableHandler[T]
}

func (l loggedHandler[T]) handle(
	ctx context.Context,
	evt asynxModels.Event[T],
) {
	err := l.handler(ctx, evt)
	if err == nil {
		return
	}
	slog.ErrorContext(ctx, "bind: handler failed",
		"event", evt.EventName,
		"aggregate", evt.AggregateID,
		"err", err,
	)
}
