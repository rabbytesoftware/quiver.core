package sqlite

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/asynx/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type eventTx struct {
	db *gorm.DB
}

func (t *eventTx) Append(
	ctx context.Context,
	streamID string,
	version int64,
	data []byte,
) error {
	res := t.db.WithContext(ctx).Create(&eventEntry{AggregateID: streamID, Version: version, Data: data})
	if errors.Is(res.Error, gorm.ErrDuplicatedKey) {
		return fmt.Errorf("%w: %w (%s, v%d)", models.ErrPipelineFailed, ErrVersionConflict, streamID, version)
	}
	if res.Error != nil {
		return fmt.Errorf("eventstore: append (%s, v%d): %w", streamID, version, res.Error)
	}
	return nil
}

func (t *eventTx) DeleteStream(
	ctx context.Context,
	streamID string,
) error {
	err := t.db.WithContext(ctx).Where("aggregate_id = ?", streamID).Delete(&eventEntry{}).Error
	if err != nil {
		return fmt.Errorf("eventstore: delete stream: %w", err)
	}
	return nil
}

func (t *eventTx) DropDeliveries(
	ctx context.Context,
	aggregateID string,
) error {
	err := t.db.WithContext(ctx).Where("aggregate_id = ?", aggregateID).Delete(&deliveryEntry{}).Error
	if err != nil {
		return fmt.Errorf("eventstore: drop deliveries: %w", err)
	}
	return nil
}

func (t *eventTx) PutReceipt(
	ctx context.Context,
	commandID string,
	r models.Receipt,
) error {
	res := t.db.WithContext(ctx).Create(&receiptEntry{
		CommandID:   commandID,
		Accepted:    r.Accepted,
		AggregateID: r.AggregateID,
		EventID:     r.EventID,
		Version:     r.Version,
		Rejection:   r.Rejection,
	})
	if errors.Is(res.Error, gorm.ErrDuplicatedKey) {
		return fmt.Errorf("%w (%s)", models.ErrDuplicateReceipt, commandID)
	}
	if res.Error != nil {
		return fmt.Errorf("eventstore: put receipt: %w", res.Error)
	}
	return nil
}

func (t *eventTx) EnqueueDelivery(
	ctx context.Context,
	aggregateID string,
	version int64,
) error {
	res := t.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&deliveryEntry{AggregateID: aggregateID, Version: version})
	if res.Error != nil {
		return fmt.Errorf("eventstore: enqueue delivery (%s, v%d): %w", aggregateID, version, res.Error)
	}
	return nil
}

// Native returns the *sql.Tx behind the transaction.
func (t *eventTx) Native() any {
	return t.db.Statement.ConnPool
}
