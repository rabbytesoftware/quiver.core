package bind

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"
)

type durableRoute[T any] struct {
	match func(string) bool
	fn    asynxModels.DurableHandler[T]
}

func (r durableRoute[T]) run(
	ctx context.Context,
	evt asynxModels.Event[T],
) error {
	if !r.match(evt.EventName) {
		return nil
	}
	return r.fn(ctx, evt)
}
