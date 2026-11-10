package events

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/asynx/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite/internal/entries"
)

// Tx is the transaction Atomically hands to its callback: every write goes
// through one database/sql transaction.
type Tx struct {
	db *gorm.DB
}

// Append writes one event, failing with ErrVersionConflict when (stream, version) is taken.
func (t *Tx) Append(
	ctx context.Context,
	streamID string,
	version int64,
	data []byte,
) error {
	res := t.db.WithContext(ctx).Create(&entries.Event{AggregateID: streamID, Version: version, Data: data})
	if errors.Is(res.Error, gorm.ErrDuplicatedKey) {
		return fmt.Errorf("%w: %w (%s, v%d)", models.ErrPipelineFailed, entries.ErrVersionConflict, streamID, version)
	}
	if res.Error != nil {
		return fmt.Errorf("eventstore: append (%s, v%d): %w", streamID, version, res.Error)
	}
	return nil
}

// DeleteStream removes every event of the stream.
func (t *Tx) DeleteStream(
	ctx context.Context,
	streamID string,
) error {
	err := t.db.WithContext(ctx).Where("aggregate_id = ?", streamID).Delete(&entries.Event{}).Error
	if err != nil {
		return fmt.Errorf("eventstore: delete stream: %w", err)
	}
	return nil
}

// DropDeliveries removes the aggregate's pending deliveries.
func (t *Tx) DropDeliveries(
	ctx context.Context,
	aggregateID string,
) error {
	err := t.db.WithContext(ctx).Where("aggregate_id = ?", aggregateID).Delete(&entries.Delivery{}).Error
	if err != nil {
		return fmt.Errorf("eventstore: drop deliveries: %w", err)
	}
	return nil
}

// PutReceipt stores the outcome of a command, failing with ErrDuplicateReceipt when one exists.
func (t *Tx) PutReceipt(
	ctx context.Context,
	commandID string,
	r models.Receipt,
) error {
	res := t.db.WithContext(ctx).Create(&entries.Receipt{
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

// EnqueueDelivery records an event as owed to the durable subscribers; doing it twice keeps one entry.
func (t *Tx) EnqueueDelivery(
	ctx context.Context,
	aggregateID string,
	version int64,
) error {
	res := t.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&entries.Delivery{AggregateID: aggregateID, Version: version})
	if res.Error != nil {
		return fmt.Errorf("eventstore: enqueue delivery (%s, v%d): %w", aggregateID, version, res.Error)
	}
	return nil
}

// Native returns the *sql.Tx behind the transaction.
func (t *Tx) Native() any {
	return t.db.Statement.ConnPool
}
