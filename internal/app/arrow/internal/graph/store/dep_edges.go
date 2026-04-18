package store

import (
	"context"
	"fmt"

	glebarez "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// DepEdgeRow is the GORM model for a dependency edge between two arrow versions.
type DepEdgeRow struct {
	FromNamespace string `gorm:"primaryKey;column:from_namespace;index:idx_dep_edges_from"`
	FromVersion   string `gorm:"primaryKey;column:from_version;index:idx_dep_edges_from"`
	ToNamespace   string `gorm:"primaryKey;column:to_namespace;index:idx_dep_edges_to"`
	ToVersion     string `gorm:"not null;column:to_version;index:idx_dep_edges_to"`
	Constraint    string `gorm:"not null;column:constraint"`
	DepType       string `gorm:"not null;column:dep_type"`
}

func (DepEdgeRow) TableName() string { return "dep_edges" }

// DepEdgeStore persists dependency edges between arrow versions.
type DepEdgeStore interface {
	Save(ctx context.Context, fromNs, fromVersion string, rows []DepEdgeRow) error
	DeleteFrom(ctx context.Context, fromNs, fromVersion string) error
	ByDependency(ctx context.Context, toNs, toVersion string) ([]DepEdgeRow, error)
	HasAnyDependents(ctx context.Context, toNs, excludeFromNs string) (bool, error)
}

type depEdgeStore struct {
	db *gorm.DB
}

// NewDepEdgeStore opens a SQLite DB at path and auto-migrates the dep_edges table.
func NewDepEdgeStore(path string) (DepEdgeStore, error) {
	db, err := gorm.Open(glebarez.Open(path), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("dep edge store: open: %w", err)
	}

	if path == ":memory:" {
		sqlDB, _ := db.DB()
		sqlDB.SetMaxOpenConns(1)
	}

	if err := db.AutoMigrate(&DepEdgeRow{}); err != nil {
		return nil, fmt.Errorf("dep edge store: migrate: %w", err)
	}

	return &depEdgeStore{db: db}, nil
}

// Save is idempotent: deletes existing edges for (fromNs, fromVersion) then batch inserts.
// If rows is empty, only the delete is performed (clearing all edges for that scope).
func (s *depEdgeStore) Save(ctx context.Context, fromNs, fromVersion string, rows []DepEdgeRow) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("from_namespace = ? AND from_version = ?", fromNs, fromVersion).
			Delete(&DepEdgeRow{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
}

// DeleteFrom removes all edges where from_namespace=fromNs AND from_version=fromVersion.
func (s *depEdgeStore) DeleteFrom(ctx context.Context, fromNs, fromVersion string) error {
	return s.db.WithContext(ctx).
		Where("from_namespace = ? AND from_version = ?", fromNs, fromVersion).
		Delete(&DepEdgeRow{}).Error
}

// ByDependency returns all edges pointing to (toNs, toVersion).
func (s *depEdgeStore) ByDependency(ctx context.Context, toNs, toVersion string) ([]DepEdgeRow, error) {
	var rows []DepEdgeRow
	err := s.db.WithContext(ctx).
		Where("to_namespace = ? AND to_version = ?", toNs, toVersion).
		Find(&rows).Error
	return rows, err
}

// HasAnyDependents returns true if any edge points to toNs from a namespace other than excludeFromNs.
func (s *depEdgeStore) HasAnyDependents(ctx context.Context, toNs, excludeFromNs string) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&DepEdgeRow{}).
		Where("to_namespace = ? AND from_namespace != ?", toNs, excludeFromNs).
		Limit(1).
		Count(&count).Error
	return count > 0, err
}
