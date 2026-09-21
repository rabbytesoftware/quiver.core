package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/char2cs/asynx/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type eventEntry struct {
	AggregateID string `gorm:"primaryKey;column:aggregate_id"`
	Version     int64  `gorm:"primaryKey;column:version"`
	Data        []byte `gorm:"not null"`
}

func (eventEntry) TableName() string {
	return "events"
}

type eventStore struct {
	db *gorm.DB
}

// NewEventStore returns a GORM-backed asynx event store.
func NewEventStore(path string) (Store, error) {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		TranslateError: true,
	})
	if err != nil {
		return nil, fmt.Errorf("eventstore: open: %w", err)
	}

	if err := prepareEventDB(db); err != nil {
		closeDB(db)
		return nil, err
	}

	return &eventStore{db: db}, nil
}

func prepareEventDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("eventstore: db: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)

	if err := db.Exec("PRAGMA busy_timeout=5000").Error; err != nil {
		return fmt.Errorf("eventstore: busy_timeout: %w", err)
	}

	if err := db.AutoMigrate(&eventEntry{}); err != nil {
		return fmt.Errorf("eventstore: migrate: %w", err)
	}

	return nil
}

func (s *eventStore) Append(
	ctx context.Context,
	aggregateID string,
	version int64,
	data []byte,
) error {
	result := s.db.WithContext(ctx).Create(&eventEntry{
		AggregateID: aggregateID,
		Version:     version,
		Data:        data,
	})
	if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
		return fmt.Errorf("%w: version conflict (%s, v%d)", models.ErrPipelineFailed, aggregateID, version)
	}
	if result.Error != nil {
		return fmt.Errorf("eventstore: append (%s, v%d): %w", aggregateID, version, result.Error)
	}
	return nil
}

func (s *eventStore) ReadFrom(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
) ([][]byte, error) {
	var entries []eventEntry
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

func (s *eventStore) ReadRange(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
	count int64,
) ([][]byte, error) {
	var entries []eventEntry
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

func (s *eventStore) Delete(
	ctx context.Context,
	aggregateID string,
) error {
	result := s.db.WithContext(ctx).
		Where("aggregate_id = ?", aggregateID).
		Delete(&eventEntry{})
	if result.Error != nil {
		return fmt.Errorf("eventstore: delete: %w", result.Error)
	}
	return nil
}

// Close closes the underlying database connection, releasing its file handle.
func (s *eventStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("eventstore: close: %w", err)
	}
	return sqlDB.Close()
}

// ListAggregateIDs returns the distinct aggregate IDs with at least one event row.
// asynx stores event keys as "events:"+id and snapshot keys as "snapshots:"+id in
// the same table, so we filter to the "events:" prefix and strip it.
func (s *eventStore) ListAggregateIDs(ctx context.Context) ([]string, error) {
	const prefix = "events:"
	var keys []string
	err := s.db.WithContext(ctx).
		Model(&eventEntry{}).
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
