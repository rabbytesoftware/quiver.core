package sqlite

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx/models"
	"gorm.io/gorm"
)

// Atomically runs fn in one SQLite transaction, rolling back everything
// written through its Tx when fn returns an error.
func (s *eventStore) Atomically(
	ctx context.Context,
	fn func(tx models.Tx) error,
) error {
	err := s.db.WithContext(ctx).Transaction(func(g *gorm.DB) error {
		return fn(&eventTx{db: g})
	})
	if err != nil {
		return fmt.Errorf("eventstore: atomically: %w", err)
	}
	return nil
}

// GetReceipt returns the receipt stored for commandID and whether there was
// one.
func (s *eventStore) GetReceipt(
	ctx context.Context,
	commandID string,
) (models.Receipt, bool, error) {
	var e receiptEntry
	res := s.db.WithContext(ctx).Where("command_id = ?", commandID).Limit(1).Find(&e)
	if res.Error != nil {
		return models.Receipt{}, false, fmt.Errorf("eventstore: get receipt: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return models.Receipt{}, false, nil
	}
	return models.Receipt{
		Accepted:    e.Accepted,
		AggregateID: e.AggregateID,
		EventID:     e.EventID,
		Version:     e.Version,
		Rejection:   e.Rejection,
	}, true, nil
}

// PendingDeliveries returns up to limit unacknowledged deliveries in enqueue
// order.
func (s *eventStore) PendingDeliveries(
	ctx context.Context,
	limit int,
) ([]models.Delivery, error) {
	var entries []deliveryEntry
	err := s.db.WithContext(ctx).Order("seq ASC").Limit(limit).Find(&entries).Error
	if err != nil {
		return nil, fmt.Errorf("eventstore: pending deliveries: %w", err)
	}
	out := make([]models.Delivery, len(entries))
	for i, e := range entries {
		out[i] = models.Delivery{AggregateID: e.AggregateID, Version: e.Version}
	}
	return out, nil
}

// AckDelivery removes one delivery; acknowledging a missing one is not an
// error.
func (s *eventStore) AckDelivery(
	ctx context.Context,
	aggregateID string,
	version int64,
) error {
	err := s.db.WithContext(ctx).
		Where("aggregate_id = ? AND version = ?", aggregateID, version).
		Delete(&deliveryEntry{}).Error
	if err != nil {
		return fmt.Errorf("eventstore: ack delivery (%s, v%d): %w", aggregateID, version, err)
	}
	return nil
}

// DB returns the events database handle; see Hosted.
func (s *eventStore) DB() *gorm.DB {
	return s.db
}
