// Package sqlite provides a GORM-backed Store[T any, K comparable] implementation.
package sqlite

import (
	"context"
	"errors"
	"fmt"

	glebarez "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"github.com/rabbytesoftware/quiver/internal/adapter/store"
)

type gormStore[T any, K comparable] struct {
	db    *gorm.DB
	pkCol string
}

// New opens (or creates) a SQLite-backed Store[T, K] at path.
// T must be a struct with GORM tags (gorm:"primaryKey" on the PK field, TableName() method).
// Pins to a single connection to prevent SQLite "database is locked" errors under concurrent access.
func New[T any, K comparable](
	path string,
) (store.Store[T, K], error) {
	db, err := gorm.Open(glebarez.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}

	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)

	var zero T
	if err := db.AutoMigrate(&zero); err != nil {
		return nil, fmt.Errorf("sqlite: migrate: %w", err)
	}

	pkCol, err := primaryKeyColumn[T](db)
	if err != nil {
		return nil, fmt.Errorf("sqlite: schema: %w", err)
	}

	return &gormStore[T, K]{db: db, pkCol: pkCol}, nil
}

// OpenDB opens (or creates) a SQLite database at path.
// Pins to a single connection to prevent SQLite "database is locked" errors under concurrent access.
func OpenDB(path string) (*gorm.DB, error) {
	db, err := gorm.Open(glebarez.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	return db, nil
}

// NewFromDB creates a Store[T, K] backed by an already-open *gorm.DB.
// Auto-migrates T's table into the provided DB.
func NewFromDB[T any, K comparable](
	db *gorm.DB,
) (store.Store[T, K], error) {
	var zero T
	if err := db.AutoMigrate(&zero); err != nil {
		return nil, fmt.Errorf("sqlite: migrate: %w", err)
	}
	pkCol, err := primaryKeyColumn[T](db)
	if err != nil {
		return nil, fmt.Errorf("sqlite: schema: %w", err)
	}
	return &gormStore[T, K]{db: db, pkCol: pkCol}, nil
}

func primaryKeyColumn[T any](
	db *gorm.DB,
) (string, error) {
	var zero T
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(&zero); err != nil {
		return "", err
	}
	namer := schema.NamingStrategy{}
	for _, field := range stmt.Schema.Fields {
		if field.PrimaryKey {
			return namer.ColumnName("", field.Name), nil
		}
	}
	return "", fmt.Errorf("no primary key field found")
}

func (s *gormStore[T, K]) Save(
	ctx context.Context,
	item T,
) error {
	return s.db.WithContext(ctx).Save(&item).Error
}

func (s *gormStore[T, K]) Delete(
	ctx context.Context,
	id K,
) error {
	var zero T
	return s.db.WithContext(ctx).Where(s.pkCol+" = ?", id).Delete(&zero).Error
}

func (s *gormStore[T, K]) FindByKey(
	ctx context.Context,
	id K,
) (*T, error) {
	var item T
	err := s.db.WithContext(ctx).Where(s.pkCol+" = ?", id).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *gormStore[T, K]) FindAll(
	ctx context.Context,
) ([]T, error) {
	var items []T
	if err := s.db.WithContext(ctx).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}
