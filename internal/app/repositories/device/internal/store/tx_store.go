package store

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

// TxStore is a Store whose Upsert can run inside an event's transaction.
type TxStore interface {
	Store

	// UpsertTx writes d through tx, so the row commits or rolls back with the
	// event.
	UpsertTx(
		ctx context.Context,
		tx asynxModels.Tx,
		d auth.Device,
	) error
}
