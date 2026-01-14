package database

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/core/database/cache"
	interfaces "github.com/rabbytesoftware/quiver/internal/core/database/interface"
	"github.com/rabbytesoftware/quiver/internal/core/database/repository"

	dberr "github.com/rabbytesoftware/quiver/internal/core/database/error"
)

type DatabaseBuilder[T any] struct {
	name        string
	cacheConfig cache.CacheConfig
	extractKey  func(entity *T) string
}

func NewDatabaseBuilder[T any](
	ctx context.Context,
	name string,
) *DatabaseBuilder[T] {
	return &DatabaseBuilder[T]{
		name: name,
	}
}

func (b *DatabaseBuilder[T]) WithCache(config cache.CacheConfig, extractKey func(entity *T) string) *DatabaseBuilder[T] {
	b.cacheConfig = config
	b.extractKey = extractKey
	return b
}

func (b *DatabaseBuilder[T]) Build() (interfaces.RepositoryInterface[T], error) {
	if b.name == "" {
		return nil, dberr.ErrNameRequired
	}

	repo, err := repository.NewRepository[T](b.name)
	if err != nil {
		return nil, err
	}

	if b.cacheConfig.IsValid() {
		return cache.NewCachedRepository[T](repo, b.cacheConfig, b.extractKey)
	}

	return repo, nil
}
