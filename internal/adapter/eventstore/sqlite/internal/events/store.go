package events

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite/internal/entries"
)

// Store is the SQLite event store: the events, the receipts and the pending
// deliveries of one asynx instance, committed together.
type Store struct {
	db *gorm.DB
}

func (s *Store) ReadFrom(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
) ([][]byte, error) {
	var entries []entries.Event
	err := s.db.WithContext(ctx).
		Where("aggregate_id = ? AND version >= ?", aggregateID, fromVersion).
		Order("version ASC").
		Find(&entries).Error
	if err != nil {
		return nil, fmt.Errorf("eventstore: read from: %w", err)
	}

	result := make([][]byte, len(entries))
	for i, e := range entries {
		result[i] = e.Data
	}
	return result, nil
}

func (s *Store) ReadRange(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
	count int64,
) ([][]byte, error) {
	var entries []entries.Event
	err := s.db.WithContext(ctx).
		Where("aggregate_id = ? AND version >= ?", aggregateID, fromVersion).
		Order("version ASC").
		Limit(int(count)).
		Find(&entries).Error
	if err != nil {
		return nil, fmt.Errorf("eventstore: read range: %w", err)
	}

	result := make([][]byte, len(entries))
	for i, e := range entries {
		result[i] = e.Data
	}
	return result, nil
}

// Close closes the underlying database connection, releasing its file handle.
func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("eventstore: close: %w", err)
	}
	return sqlDB.Close()
}

// ListAggregateIDs returns the distinct aggregate IDs with at least one event row.
// asynx stores event keys as "events:"+id and snapshot keys as "snapshots:"+id in
// the same table, so we filter to the "events:" prefix and strip it.
func (s *Store) ListAggregateIDs(ctx context.Context) ([]string, error) {
	const prefix = "events:"
	var keys []string
	err := s.db.WithContext(ctx).
		Model(&entries.Event{}).
		Distinct("aggregate_id").
		Where("aggregate_id LIKE ?", prefix+"%").
		Pluck("aggregate_id", &keys).Error
	if err != nil {
		return nil, fmt.Errorf("eventstore: list aggregate ids: %w", err)
	}

	ids := make([]string, 0, len(keys))
	for _, k := range keys {
		ids = append(ids, strings.TrimPrefix(k, prefix))
	}
	return ids, nil
}
