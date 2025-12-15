package database

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/core/database/cache"
	interfaces "github.com/rabbytesoftware/quiver/internal/core/database/interface"
	"github.com/rabbytesoftware/quiver/internal/core/database/repository"
)

// DatabaseBuilder provides a builder pattern for creating database repositories with optional caching.
type DatabaseBuilder[T any] struct {
	ctx         context.Context
	name        string
	cacheConfig *cache.CacheConfig
}

// NewDatabaseBuilder creates a new database builder.
func NewDatabaseBuilder[T any](
	ctx context.Context,
	name string,
) *DatabaseBuilder[T] {
	return &DatabaseBuilder[T]{
		ctx:  ctx,
		name: name,
	}
}

// WithCache configures the builder to use caching with the provided configuration.
func (b *DatabaseBuilder[T]) WithCache(config cache.CacheConfig) *DatabaseBuilder[T] {
	b.cacheConfig = &config
	return b
}

// Build creates and returns a repository interface, optionally wrapped with caching.
func (b *DatabaseBuilder[T]) Build() (interfaces.RepositoryInterface[T], error) {
	// Create base repository
	baseRepo, err := repository.NewRepository[T](b.name)
	if err != nil {
		return nil, err
	}

	// If cache is not configured or disabled, return base repository
	if b.cacheConfig == nil || !b.cacheConfig.Enabled {
		return baseRepo, nil
	}

	// Create cache instance
	cacheInstance := cache.NewGoCache(*b.cacheConfig)
	if cacheInstance == nil {
		// Cache creation failed (likely disabled), return base repository
		return baseRepo, nil
	}

	// Wrap repository with cache
	return cache.NewRepositoryCache(baseRepo, cacheInstance, *b.cacheConfig), nil
}

