package binder

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"
)

// SyncFn is the body of a sync projection: it runs inside the transaction that
// stores the event, so everything it writes through tx commits or rolls back
// with that event.
type SyncFn[T any] func(
	ctx context.Context,
	tx asynxModels.Tx,
	evt asynxModels.Event[T],
) error
