package store

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PendingRow is one arrow namespace whose runtime aggregate still needs
// forgetting after the arrow itself was removed.
type PendingRow struct {
	Namespace  string    `gorm:"primaryKey;column:namespace"`
	EnqueuedAt time.Time `gorm:"column:enqueued_at"`
}

func (PendingRow) TableName() string { return "forget_cascade_pending" }

type Store interface {
	Enqueue(ctx context.Context, ns string) error
	Pending(ctx context.Context) ([]string, error)
	Complete(ctx context.Context, ns string) error
}

type pendingStore struct {
	db *gorm.DB
}

func New(db *gorm.DB) (Store, error) {
	if db == nil {
		return nil, fmt.Errorf("forget cascade store: db must not be nil")
	}
	if err := db.AutoMigrate(&PendingRow{}); err != nil {
		return nil, fmt.Errorf("forget cascade store: migrate: %w", err)
	}
	return &pendingStore{db: db}, nil
}

// Enqueue is idempotent: a namespace already pending is left as-is rather than
// erroring, since two removals racing the same namespace must both succeed.
func (s *pendingStore) Enqueue(ctx context.Context, ns string) error {
	return s.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&PendingRow{Namespace: ns, EnqueuedAt: time.Now()}).Error
}

func (s *pendingStore) Pending(ctx context.Context) ([]string, error) {
	var rows []PendingRow
	if err := s.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]string, len(rows))
	for i, r := range rows {
		result[i] = r.Namespace
	}
	return result, nil
}

func (s *pendingStore) Complete(ctx context.Context, ns string) error {
	return s.db.WithContext(ctx).Delete(&PendingRow{}, "namespace = ?", ns).Error
}
