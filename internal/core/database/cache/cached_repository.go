package cache

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/patrickmn/go-cache"
	cacheerr "github.com/rabbytesoftware/quiver/internal/core/database/cache/error"
	interfaces "github.com/rabbytesoftware/quiver/internal/core/database/interface"
	"github.com/rabbytesoftware/quiver/internal/core/watcher"
)

type CachedRepository[T any] struct {
	db         interfaces.RepositoryInterface[T]
	cache      *cache.Cache
	config     CacheConfig
	ExtractKey func(entity *T) (string, error)
}

func NewCachedRepository[T any](
	baseRepo interfaces.RepositoryInterface[T],
	config CacheConfig,
	ExtractKey func(entity *T) (string, error),
) (interfaces.RepositoryInterface[T], error) {

	if baseRepo == nil {
		return nil, cacheerr.ErrMissingBase
	}

	if config.DefaultTTL <= 0 || config.CleanupInterval <= 0 {
		return nil, cacheerr.ErrInvalidCacheConfig
	}

	return &CachedRepository[T]{
		db:         baseRepo,
		cache:      cache.New(config.DefaultTTL, config.CleanupInterval),
		config:     config,
		ExtractKey: ExtractKey,
	}, nil
}

func (cr *CachedRepository[T]) Create(ctx context.Context, entity *T) (*T, error) {
	return cr.db.Create(ctx, entity)
}

func (cr *CachedRepository[T]) Get(ctx context.Context) ([]*T, error) {
	key := cr.buildListKey()

	v, found := cr.cache.Get(key)

	if found {
		data, ok := v.([]byte)
		if !ok {
			return nil, cacheerr.ErrInvalidCacheValue
		}
		var result []*T
		err := json.Unmarshal(data, &result)
		if err != nil {
			return nil, err
		}
		return result, nil
	}

	result, err := cr.db.Get(ctx)
	if err != nil {
		return nil, err
	}

	if err := cr.setList(key, result); err != nil {
		watcher.Warn(fmt.Sprintf("%v: failed to cache list: %v",
			cacheerr.ErrCacheWriteFailed, err))
	}

	return result, nil
}

func (cr *CachedRepository[T]) GetByID(ctx context.Context, id uuid.UUID) (*T, error) {
	key := cr.buildEntityKey(id.String())
	v, found := cr.cache.Get(key)
	if found {
		data, ok := v.([]byte)
		if !ok {
			return nil, cacheerr.ErrInvalidCacheValue
		}
		var result *T
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, err
		}

		return result, nil
	}

	entity, err := cr.db.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := cr.set(key, entity); err != nil {
		watcher.Warn(fmt.Sprintf("%v: failed to cache entity %s: %v",
			cacheerr.ErrCacheWriteFailed, id, err))
	}

	return entity, nil
}

func (cr *CachedRepository[T]) Update(ctx context.Context, entity *T) (*T, error) {
	result, err := cr.db.Update(ctx, entity)
	if err != nil {
		return nil, err
	}

	id, err := cr.ExtractKey(result)
	if err != nil {
		return result, cacheerr.ErrIDExtractionFailed
	}

	key := cr.buildEntityKey(id)
	cr.cache.Delete(key)

	return result, nil
}

func (cr *CachedRepository[T]) Delete(ctx context.Context, id uuid.UUID) error {
	if err := cr.db.Delete(ctx, id); err != nil {
		return err
	}

	key := cr.buildEntityKey(id.String())
	cr.cache.Delete(key)

	return nil
}

func (cr *CachedRepository[T]) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	if _, found := cr.cache.Get(cr.buildEntityKey(id.String())); found {
		return true, nil
	}

	return cr.db.Exists(ctx, id)
}

func (cr *CachedRepository[T]) Count(ctx context.Context) (int64, error) {
	if count, found := cr.cache.Get(cr.buildListKey()); found {
		return count.(int64), nil
	}

	return cr.db.Count(ctx)
}

func (cr *CachedRepository[T]) Where(
	ctx context.Context,
	query string,
	args ...interface{},
) ([]*T, error) {
	return cr.db.Where(ctx, query, args...)
}

func (cr *CachedRepository[T]) Close() error {
	return cr.db.Close()
}

func (cr *CachedRepository[T]) buildEntityKey(id string) string {
	return fmt.Sprintf("entity:%s:%s", cr.getEntityTypeName(), id)
}

func (cr *CachedRepository[T]) buildListKey() string {
	return fmt.Sprintf("list:%s", cr.getEntityTypeName())
}

func (cr *CachedRepository[T]) getEntityTypeName() string {
	var zero T
	return fmt.Sprintf("%T", zero)
}

func (cr *CachedRepository[T]) set(key string, data *T) error {
	value, err := json.Marshal(data)
	if err != nil {
		return err
	}

	cr.cache.Set(key, value, cr.config.DefaultTTL)
	return nil
}

func (cr *CachedRepository[T]) setList(key string, data []*T) error {
	value, err := json.Marshal(data)
	if err != nil {
		return err
	}

	cr.cache.Set(key, value, cr.config.DefaultTTL)
	return nil
}
