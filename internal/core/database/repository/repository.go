package repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/rabbytesoftware/quiver/internal/core/config"
	interfaces "github.com/rabbytesoftware/quiver/internal/core/database/interface"
)

type Repository[T any] struct {
	db   *gorm.DB
	name string
}

func NewRepository[T any](
	name string,
) (interfaces.RepositoryInterface[T], error) {
	dbConfig := config.GetDatabase()

	dbPath := dbConfig.Path
	if envPath := os.Getenv("QUIVER_DATABASE_PATH"); envPath != "" {
		dbPath = envPath
	}

	dbPath = filepath.Join(dbPath, fmt.Sprintf("%s.db", name))

	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := db.AutoMigrate(new(T)); err != nil {
		return nil, fmt.Errorf("failed to migrate database schema: %w", err)
	}

	return &Repository[T]{
		db:   db,
		name: name,
	}, nil
}

func (r *Repository[T]) Get(
	ctx context.Context,
) ([]*T, error) {
	var entities []*T
	if err := r.db.WithContext(ctx).Find(&entities).Error; err != nil {
		return nil, fmt.Errorf("failed to get entities: %w", err)
	}
	return entities, nil
}

func (r *Repository[T]) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (*T, error) {
	var entity T
	if err := r.db.WithContext(ctx).First(&entity, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("entity with id %s not found", id)
		}
		return nil, fmt.Errorf("failed to get entity by id: %w", err)
	}
	return &entity, nil
}

func (r *Repository[T]) Create(
	ctx context.Context,
	entity *T,
) (*T, error) {
	if err := r.db.WithContext(ctx).Create(entity).Error; err != nil {
		return nil, fmt.Errorf("failed to create entity: %w", err)
	}
	return entity, nil
}

func (r *Repository[T]) Update(
	ctx context.Context,
	entity *T,
) (*T, error) {
	if err := r.db.WithContext(ctx).Save(entity).Error; err != nil {
		return nil, fmt.Errorf("failed to update entity: %w", err)
	}
	return entity, nil
}

func (r *Repository[T]) Delete(
	ctx context.Context,
	id uuid.UUID,
) error {
	if err := r.db.WithContext(ctx).Delete(new(T), "id = ?", id).Error; err != nil {
		return fmt.Errorf("failed to delete entity: %w", err)
	}
	return nil
}

func (r *Repository[T]) Exists(
	ctx context.Context,
	id uuid.UUID,
) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(new(T)).Where("id = ?", id).Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check entity existence: %w", err)
	}
	return count > 0, nil
}

func (r *Repository[T]) Count(
	ctx context.Context,
) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(new(T)).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count entities: %w", err)
	}
	return count, nil
}
