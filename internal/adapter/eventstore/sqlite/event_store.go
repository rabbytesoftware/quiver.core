package sqlite

import (
	"context"
	"fmt"

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
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("eventstore: open: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("eventstore: db: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)

	if err := db.AutoMigrate(&eventEntry{}); err != nil {
		return nil, fmt.Errorf("eventstore: migrate: %w", err)
	}

	return &eventStore{db: db}, nil
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
	if result.Error != nil {
		return fmt.Errorf("%w: version conflict (%s, v%d)", models.ErrPipelineFailed, aggregateID, version)
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

func (s *eventStore) Count(
	ctx context.Context,
	aggregateID string,
	fromVersion int64,
) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).
		Model(&eventEntry{}).
		Where("aggregate_id = ? AND version >= ?", aggregateID, fromVersion).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("eventstore: count: %w", err)
	}
	return count, nil
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

// Close closes the underlying database connection, checkpointing the WAL and releasing file handles.
func (s *eventStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("eventstore: close: %w", err)
	}
	return sqlDB.Close()
}
