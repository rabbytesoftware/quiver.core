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
	cacheConfig *cache.CacheConfig
}

func NewDatabaseBuilder[T any](
	ctx context.Context,
	name string,
) *DatabaseBuilder[T] {
	return &DatabaseBuilder[T]{
		name: name,
	}
}

func (b *DatabaseBuilder[T]) WithCache(config cache.CacheConfig) *DatabaseBuilder[T] {
	b.cacheConfig = &config
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

	if b.cacheConfig != nil && b.cacheConfig.IsValid() {
		return cache.NewCachedRepository[T](repo, *b.cacheConfig)
	}

	return repo, nil
}
