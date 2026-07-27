package store

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

type DepEdgeRow struct {
	FromNamespace string `gorm:"primaryKey;column:from_namespace;index:idx_graph_dep_edges_from"`
	FromVersion   string `gorm:"primaryKey;column:from_version;index:idx_graph_dep_edges_from"`
	ToNamespace   string `gorm:"primaryKey;column:to_namespace;index:idx_graph_dep_edges_to"`
	ToVersion     string `gorm:"not null;column:to_version;index:idx_graph_dep_edges_to"`
	Constraint    string `gorm:"not null;column:constraint"`
	DepType       string `gorm:"not null;column:dep_type"`
}

func (DepEdgeRow) TableName() string { return "graph_dep_edges" }

type DepEdgeStore interface {
	Save(
		ctx context.Context,
		fromNs, fromVersion string,
		rows []DepEdgeRow,
	) error

	DeleteFrom(
		ctx context.Context,
		fromNs, fromVersion string,
	) error

	// DeleteTo removes the edges pointing at one namespace@ref. The ref is part
	// of the key because a row names the ref it depends on, and two refs of the
	// same namespace are two different dependencies.
	DeleteTo(
		ctx context.Context,
		toNs, toVersion string,
	) error

	ByDependency(
		ctx context.Context,
		toNs, toVersion string,
	) ([]DepEdgeRow, error)

	EdgesToBare(
		ctx context.Context,
		toNs string,
	) ([]DepEdgeRow, error)

	HasAnyDependents(
		ctx context.Context,
		toNs, excludeFromNs string,
	) (bool, error)
}

type depEdgeStore struct {
	db *gorm.DB
}

func NewDepEdgeStore(
	db *gorm.DB,
) (DepEdgeStore, error) {
	if db == nil {
		return nil, fmt.Errorf("graph dep edge store: db must not be nil")
	}

	if err := db.AutoMigrate(&DepEdgeRow{}); err != nil {
		return nil, fmt.Errorf("graph dep edge store: migrate: %w", err)
	}

	return &depEdgeStore{db: db}, nil
}

func (s *depEdgeStore) Save(
	ctx context.Context,
	fromNs, fromVersion string,
	rows []DepEdgeRow,
) error {
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

func (s *depEdgeStore) DeleteFrom(
	ctx context.Context,
	fromNs, fromVersion string,
) error {
	return s.db.WithContext(ctx).
		Where("from_namespace = ? AND from_version = ?", fromNs, fromVersion).
		Delete(&DepEdgeRow{}).Error
}

func (s *depEdgeStore) DeleteTo(
	ctx context.Context,
	toNs, toVersion string,
) error {
	return s.db.WithContext(ctx).
		Where("to_namespace = ? AND to_version = ?", toNs, toVersion).
		Delete(&DepEdgeRow{}).Error
}

func (s *depEdgeStore) ByDependency(
	ctx context.Context,
	toNs, toVersion string,
) ([]DepEdgeRow, error) {
	var rows []DepEdgeRow
	err := s.db.WithContext(ctx).
		Where("to_namespace = ? AND to_version = ?", toNs, toVersion).
		Find(&rows).Error
	return rows, err
}

func (s *depEdgeStore) EdgesToBare(
	ctx context.Context,
	toNs string,
) ([]DepEdgeRow, error) {
	var rows []DepEdgeRow
	err := s.db.WithContext(ctx).
		Where("to_namespace = ?", toNs).
		Find(&rows).Error
	return rows, err
}

func (s *depEdgeStore) HasAnyDependents(
	ctx context.Context,
	toNs, excludeFromNs string,
) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&DepEdgeRow{}).
		Where("to_namespace = ? AND from_namespace != ?", toNs, excludeFromNs).
		Limit(1).
		Count(&count).Error
	return count > 0, err
}
