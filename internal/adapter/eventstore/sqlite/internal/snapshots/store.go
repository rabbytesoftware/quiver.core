package snapshots

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/rabbytesoftware/quiver.core/internal/adapter/eventstore/sqlite/internal/entries"
)

// Store is the SQLite snapshot store: one upserted row per aggregate.
type Store struct {
	db *gorm.DB
}

// Put stores the snapshot for aggregateID, replacing any snapshot already
// stored for it. The upsert is keyed on aggregate_id alone and guarded by a
// monotonicity check so a stale write never clobbers a newer snapshot; a
// write that loses the guard affects zero rows and returns no error.
func (s *Store) Put(
	ctx context.Context,
	aggregateID string,
	version int64,
	data []byte,
) error {
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "aggregate_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"version": gorm.Expr("excluded.version"),
			"data":    gorm.Expr("excluded.data"),
		}),
		Where: clause.Where{Exprs: []clause.Expression{
			gorm.Expr("excluded.version > snapshots.version"),
		}},
	}).Create(&entries.Snapshot{AggregateID: aggregateID, Version: version, Data: data}).Error
	if err != nil {
		return fmt.Errorf("snapshotstore: put (%s, v%d): %w", aggregateID, version, err)
	}
	return nil
}

// Get returns the stored snapshot for aggregateID. found is false when no
// snapshot exists, which is the normal state for an aggregate that has never
// been snapshotted.
func (s *Store) Get(
	ctx context.Context,
	aggregateID string,
) ([]byte, bool, error) {
	var entry entries.Snapshot
	err := s.db.WithContext(ctx).
		Where("aggregate_id = ?", aggregateID).
		First(&entry).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("snapshotstore: get (%s): %w", aggregateID, err)
	}
	return entry.Data, true, nil
}

// Delete removes the snapshot for aggregateID. Idempotent — deleting a
// non-existent aggregateID is not an error.
func (s *Store) Delete(
	ctx context.Context,
	aggregateID string,
) error {
	result := s.db.WithContext(ctx).
		Where("aggregate_id = ?", aggregateID).
		Delete(&entries.Snapshot{})
	if result.Error != nil {
		return fmt.Errorf("snapshotstore: delete (%s): %w", aggregateID, result.Error)
	}
	return nil
}

// Close closes the underlying database connection, releasing its file handle.
func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("snapshotstore: close: %w", err)
	}
	return sqlDB.Close()
}
