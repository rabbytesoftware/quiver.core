package binder

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"
)

type syncRoute[T any] struct {
	match func(string) bool
	fn    SyncFn[T]
}

func (r syncRoute[T]) run(
	ctx context.Context,
	tx asynxModels.Tx,
	evt asynxModels.Event[T],
) error {
	if !r.match(evt.EventName) {
		return nil
	}
	return r.fn(ctx, tx, evt)
}
