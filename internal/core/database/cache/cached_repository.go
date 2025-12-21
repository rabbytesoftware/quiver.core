package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/google/uuid"

	"github.com/patrickmn/go-cache"
	cacheerr "github.com/rabbytesoftware/quiver/internal/core/database/cache/error"
	interfaces "github.com/rabbytesoftware/quiver/internal/core/database/interface"
)

type CachedRepository[T any] struct {
	base   interfaces.RepositoryInterface[T]
	cache  *cache.Cache
	config CacheConfig
}

func NewCachedRepository[T any](
	baseRepo interfaces.RepositoryInterface[T],
	config CacheConfig,
) (interfaces.RepositoryInterface[T], error) {
	if !config.Enabled {
		return baseRepo, nil
	}

	if !config.IsValid() {
		return baseRepo, cacheerr.ErrInvalidCacheConfig
	}

	return &CachedRepository[T]{
		base:   baseRepo,
		cache:  cache.New(config.DefaultTTL, config.CleanupInterval),
		config: config,
	}, nil
}

func (cr *CachedRepository[T]) Create(ctx context.Context, entity *T) (*T, error) {
	if cr.base == nil {
		return nil, cacheerr.ErrMissingBase
	}

	created, err := cr.base.Create(ctx, entity)
	if err != nil {
		return nil, err
	}

	return created, nil
}

func (cr *CachedRepository[T]) Get(ctx context.Context) ([]*T, error) {
	if cr.cache == nil {
		return nil, cacheerr.ErrMissingCache
	}

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

	if cr.base == nil {
		return nil, cacheerr.ErrMissingBase
	}

	result, err := cr.base.Get(ctx)
	if err != nil {
		return nil, err
	}

	for _, entity := range result {
		if id, ok := cr.extractID(entity); ok {
			cr.set(id, entity)
		}
	}

	return result, nil
}

func (cr *CachedRepository[T]) GetByID(ctx context.Context, id uuid.UUID) (*T, error) {

	if cr.cache == nil {
		return nil, cacheerr.ErrMissingCache
	}

	key := cr.buildEntityKey(id)
	v, found := cr.cache.Get(key)

	if found {
		data, ok := v.([]byte)
		if !ok {
			return nil, cacheerr.ErrInvalidCacheValue
		}
		var result *T
		err := json.Unmarshal(data, &result)
		if err != nil {
			return nil, err
		}
		return result, nil
	}

	if cr.base == nil {
		return nil, cacheerr.ErrMissingBase
	}

	entity, err := cr.base.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if id, ok := cr.extractID(entity); ok {
		cr.set(id, entity)
	}

	return entity, nil
}

func (cr *CachedRepository[T]) Update(ctx context.Context, entity *T) (*T, error) {
	if cr.base == nil {
		return nil, cacheerr.ErrMissingBase
	}

	result, err := cr.base.Update(ctx, entity)
	if err != nil {
		return nil, err
	}

	if cr.cache == nil {
		return result, cacheerr.ErrMissingCache
	}

	id, ok := cr.extractID(result)
	if !ok {
		return result, cacheerr.ErrIDExtractionFailed
	}

	key := cr.buildEntityKey(id)
	cr.cache.Delete(key)

	return result, nil
}

func (cr *CachedRepository[T]) Delete(ctx context.Context, id uuid.UUID) error {
	if cr.base == nil {
		return cacheerr.ErrMissingBase
	}

	if err := cr.base.Delete(ctx, id); err != nil {
		return err
	}

	if cr.cache == nil {
		return cacheerr.ErrMissingCache
	}

	key := cr.buildEntityKey(id)
	cr.cache.Delete(key)

	return nil
}

func (cr *CachedRepository[T]) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	return cr.base.Exists(ctx, id)
}

func (cr *CachedRepository[T]) Count(ctx context.Context) (int64, error) {
	return cr.base.Count(ctx)
}

func (cr *CachedRepository[T]) Close() error {
	return cr.base.Close()
}

func (cr *CachedRepository[T]) extractID(entity *T) (uuid.UUID, bool) {
	if entity == nil {
		return uuid.Nil, false
	}

	val := reflect.ValueOf(entity)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return uuid.Nil, false
	}

	idField := val.FieldByName("ID")
	if !idField.IsValid() {
		return uuid.Nil, false
	}

	if id, ok := idField.Interface().(uuid.UUID); ok {
		return id, true
	}

	return uuid.Nil, false
}

func (cr *CachedRepository[T]) buildEntityKey(id uuid.UUID) string {
	return fmt.Sprintf("entity:%s:%s", cr.getEntityTypeName(), id.String())
}

func (cr *CachedRepository[T]) buildListKey() string {
	return fmt.Sprintf("list:%s", cr.getEntityTypeName())
}

func (cr *CachedRepository[T]) getEntityTypeName() string {
	var zero T
	return fmt.Sprintf("%T", zero)
}

func (cr *CachedRepository[T]) set(id uuid.UUID, data *T) error {
	if cr.cache == nil {
		return cacheerr.ErrMissingCache
	}

	value, err := json.Marshal(data)
	if err != nil {
		return err
	}

	key := cr.buildEntityKey(id)
	cr.cache.Set(key, value, cr.config.DefaultTTL)
	return nil
}
