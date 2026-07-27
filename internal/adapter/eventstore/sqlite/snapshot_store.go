package sqlite

import (
	"context"
	"errors"
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type snapshotEntry struct {
	AggregateID string `gorm:"primaryKey;column:aggregate_id"`
	Version     int64  `gorm:"not null;column:version"`
	Data        []byte `gorm:"not null;column:data"`
}

func (snapshotEntry) TableName() string {
	return "snapshots"
}

type snapshotStore struct {
	db *gorm.DB
}

// NewSnapshotStore returns a GORM-backed asynx snapshot store, opened against
// its own SQLite file rather than sharing the event store's.
//
// The event store is opened single-connection and write-pinned, and
// reader.Load issues SnapshotStore.Get first on every read. A snapshots table
// living in that same database would serialize the O(1) Get behind any
// in-flight Append. asynx never wraps the event Append and the snapshot Put
// in a shared transaction, so co-locating the tables would not buy atomicity
// in exchange for that cost.
func NewSnapshotStore(path string) (SnapshotStore, error) {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		TranslateError: true,
	})
	if err != nil {
		return nil, fmt.Errorf("snapshotstore: open: %w", err)
	}

	if err := prepareSnapshotDB(db); err != nil {
		closeDB(db)
		return nil, err
	}

	return &snapshotStore{db: db}, nil
}

func prepareSnapshotDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("snapshotstore: db: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)

	if err := db.Exec("PRAGMA busy_timeout=5000").Error; err != nil {
		return fmt.Errorf("snapshotstore: busy_timeout: %w", err)
	}

	if err := db.AutoMigrate(&snapshotEntry{}); err != nil {
		return fmt.Errorf("snapshotstore: migrate: %w", err)
	}

	return nil
}

// Put stores the snapshot for aggregateID, replacing any snapshot already
// stored for it. The upsert is keyed on aggregate_id alone and guarded by a
// monotonicity check so a stale write never clobbers a newer snapshot; a
// write that loses the guard affects zero rows and returns no error.
func (s *snapshotStore) Put(
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
	}).Create(&snapshotEntry{AggregateID: aggregateID, Version: version, Data: data}).Error
	if err != nil {
		return fmt.Errorf("snapshotstore: put (%s, v%d): %w", aggregateID, version, err)
	}
	return nil
}

// Get returns the stored snapshot for aggregateID. found is false when no
// snapshot exists, which is the normal state for an aggregate that has never
// been snapshotted.
func (s *snapshotStore) Get(
	ctx context.Context,
	aggregateID string,
) ([]byte, bool, error) {
	var entry snapshotEntry
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
func (s *snapshotStore) Delete(
	ctx context.Context,
	aggregateID string,
) error {
	result := s.db.WithContext(ctx).
		Where("aggregate_id = ?", aggregateID).
		Delete(&snapshotEntry{})
	if result.Error != nil {
		return fmt.Errorf("snapshotstore: delete (%s): %w", aggregateID, result.Error)
	}
	return nil
}

// Close closes the underlying database connection, releasing its file handle.
func (s *snapshotStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("snapshotstore: close: %w", err)
	}
	return sqlDB.Close()
}
