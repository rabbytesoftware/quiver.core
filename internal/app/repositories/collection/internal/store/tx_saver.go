package store

import (
	"context"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// TxSaver is a QuiverStore whose Save can run inside an event's transaction.
type TxSaver interface {
	// SaveTx writes coll through tx, so the row commits or rolls back with
	// the event.
	SaveTx(
		ctx context.Context,
		tx asynxModels.Tx,
		coll domain.Collection,
	) error
}
