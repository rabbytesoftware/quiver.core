package cache

import (
	"context"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/google/uuid"

	cacheerr "github.com/rabbytesoftware/quiver/internal/core/database/cache/error"
	interfaces "github.com/rabbytesoftware/quiver/internal/core/database/interface"
)

type CachedRepository[T any] struct {
	db         interfaces.RepositoryInterface[T]
	cache      *ristretto.Cache[string, T]
	config     CacheConfig
	ExtractKey func(entity *T) string
}

func NewCachedRepository[T any](
	baseRepo interfaces.RepositoryInterface[T],
	config CacheConfig,
	ExtractKey func(entity *T) string,
) (interfaces.RepositoryInterface[T], error) {

	if baseRepo == nil {
		return nil, cacheerr.ErrMissingBase
	}

	if !config.IsValid() {
		return nil, cacheerr.ErrInvalidCacheConfig
	}

	ristrettoConfig := &ristretto.Config[string, T]{
		NumCounters: config.NumCounters,
		MaxCost:     config.MaxCost,
		BufferItems: config.BufferItems,
	}

	cache, err := ristretto.NewCache[string, T](ristrettoConfig)
	if err != nil {
		return nil, err
	}

	cachedRepoConfig := CacheConfig{
		NumCounters:       config.NumCounters,
		MaxCost:           config.MaxCost,
		BufferItems:       config.BufferItems,
		DefaultTTL:        config.DefaultTTL,
		DefaultCostOfItem: config.DefaultCostOfItem,
	}

	return &CachedRepository[T]{
		db:         baseRepo,
		cache:      cache,
		config:     cachedRepoConfig,
		ExtractKey: ExtractKey,
	}, nil
}

func (cr *CachedRepository[T]) Create(ctx context.Context, entity *T) (*T, error) {
	result, err := cr.db.Create(ctx, entity)
	if err != nil {
		return nil, err
	}

	go func() {
		key := cr.ExtractKey(entity)
		cr.cache.SetWithTTL(key, *result, cr.config.DefaultCostOfItem, cr.config.DefaultTTL)
	}()

	return result, nil
}

func (cr *CachedRepository[T]) GetByID(ctx context.Context, id uuid.UUID) (*T, error) {
	key := id.String()

	entity, found := cr.cache.Get(key)

	if found {
		return &entity, nil
	}

	result, err := cr.db.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	go cr.cache.SetWithTTL(key, *result, cr.config.DefaultCostOfItem, cr.config.DefaultTTL)

	return result, nil
}

func (cr *CachedRepository[T]) Update(ctx context.Context, entity *T) (*T, error) {
	result, err := cr.db.Update(ctx, entity)
	if err != nil {
		return nil, err
	}

	key := cr.ExtractKey(entity)

	go func() {
		cr.cache.Del(key)
		cr.cache.SetWithTTL(key, *entity, cr.config.DefaultCostOfItem, cr.config.DefaultTTL)
	}()

	return result, nil
}

func (cr *CachedRepository[T]) Delete(ctx context.Context, id uuid.UUID) error {
	if err := cr.db.Delete(ctx, id); err != nil {
		return err
	}

	key := id.String()
	go cr.cache.Del(key)

	return nil
}

func (cr *CachedRepository[T]) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	key := id.String()
	_, found := cr.cache.Get(key)

	if found {
		return true, nil
	}

	return cr.db.Exists(ctx, id)
}

func (cr *CachedRepository[T]) Count(ctx context.Context) (int64, error) {
	return cr.db.Count(ctx)
}

func (cr *CachedRepository[T]) Where(
	ctx context.Context,
	query string,
	args ...interface{},
) ([]*T, error) {
	return cr.db.Where(ctx, query, args...)
}

func (cr *CachedRepository[T]) WaitForCache() {
	cr.cache.Wait()
}

func (cr *CachedRepository[T]) Close() error {
	err := cr.db.Close()
	if err != nil {
		return err
	}

	cr.cache.Close()
	return nil
}
