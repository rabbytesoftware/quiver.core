package cache

import (
	"context"
	"fmt"
	"reflect"

	"github.com/google/uuid"

	interfaces "github.com/rabbytesoftware/quiver/internal/core/database/interface"
)

// RepositoryCache wraps a repository interface with caching functionality.
// It implements the decorator pattern, transparently adding caching to all read operations
// and invalidating cache on write operations.
type RepositoryCache[T any] struct {
	base   interfaces.RepositoryInterface[T]
	cache  Cache
	config CacheConfig
}

// NewRepositoryCache creates a new cached repository wrapper.
func NewRepositoryCache[T any](
	base interfaces.RepositoryInterface[T],
	cache Cache,
	config CacheConfig,
) interfaces.RepositoryInterface[T] {
	if !config.Enabled || cache == nil {
		return base // Return unwrapped repository if cache is disabled
	}

	return &RepositoryCache[T]{
		base:   base,
		cache:  cache,
		config: config,
	}
}

// buildCacheKey creates a cache key for an entity ID.
func (r *RepositoryCache[T]) buildEntityKey(id uuid.UUID) string {
	return fmt.Sprintf("entity:%s:%s", r.getEntityTypeName(), id.String())
}

// buildCacheKey creates a cache key for the Get() operation.
func (r *RepositoryCache[T]) buildListKey() string {
	return fmt.Sprintf("list:%s", r.getEntityTypeName())
}

// getEntityTypeName returns a string representation of the entity type.
func (r *RepositoryCache[T]) getEntityTypeName() string {
	var zero T
	return fmt.Sprintf("%T", zero)
}

// Get retrieves all entities, checking cache first.
func (r *RepositoryCache[T]) Get(ctx context.Context) ([]*T, error) {
	key := r.buildListKey()

	// Try cache first
	var cached []*T
	found, err := r.cache.Get(ctx, key, &cached)
	if err == nil && found {
		return cached, nil
	}

	// Cache miss - fetch from base repository
	entities, err := r.base.Get(ctx)
	if err != nil {
		return nil, err
	}

	// Cache the result
	if err := r.cache.Set(ctx, key, entities, r.config.DefaultTTL); err != nil {
		// Log error but don't fail the operation
		_ = err
	}

	return entities, nil
}

// GetByID retrieves an entity by ID, checking cache first.
func (r *RepositoryCache[T]) GetByID(ctx context.Context, id uuid.UUID) (*T, error) {
	key := r.buildEntityKey(id)

	// Try cache first
	var cached T
	found, err := r.cache.Get(ctx, key, &cached)
	if err == nil && found {
		return &cached, nil
	}

	// Cache miss - fetch from base repository
	entity, err := r.base.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Cache the result
	if err := r.cache.Set(ctx, key, entity, r.config.DefaultTTL); err != nil {
		// Log error but don't fail the operation
		_ = err
	}

	return entity, nil
}

// Create creates a new entity and invalidates the list cache.
func (r *RepositoryCache[T]) Create(ctx context.Context, entity *T) (*T, error) {
	created, err := r.base.Create(ctx, entity)
	if err != nil {
		return nil, err
	}

	// Invalidate list cache
	_ = r.cache.Delete(ctx, r.buildListKey())

	return created, nil
}

// extractID extracts the ID field from an entity using reflection.
func (r *RepositoryCache[T]) extractID(entity *T) (uuid.UUID, bool) {
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

	if idField.Kind() == reflect.Interface {
		idValue := idField.Interface()
		if id, ok := idValue.(uuid.UUID); ok {
			return id, true
		}
	}

	return uuid.Nil, false
}

// Update updates an entity and invalidates its cache entry and list cache.
func (r *RepositoryCache[T]) Update(ctx context.Context, entity *T) (*T, error) {
	updated, err := r.base.Update(ctx, entity)
	if err != nil {
		return nil, err
	}

	// Extract ID from entity using reflection
	id, ok := r.extractID(updated)
	if !ok {
		// Fallback: invalidate all caches if we can't extract ID
		_ = r.cache.Flush(ctx)
		return updated, nil
	}

	// Invalidate entity cache and list cache
	_ = r.cache.Delete(ctx, r.buildEntityKey(id))
	_ = r.cache.Delete(ctx, r.buildListKey())

	return updated, nil
}

// Delete deletes an entity and invalidates its cache entry and list cache.
func (r *RepositoryCache[T]) Delete(ctx context.Context, id uuid.UUID) error {
	err := r.base.Delete(ctx, id)
	if err != nil {
		return err
	}

	// Invalidate entity cache and list cache
	_ = r.cache.Delete(ctx, r.buildEntityKey(id))
	_ = r.cache.Delete(ctx, r.buildListKey())

	return nil
}

// Exists checks if an entity exists. This operation is not cached.
func (r *RepositoryCache[T]) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	return r.base.Exists(ctx, id)
}

// Count returns the count of entities. This operation is not cached.
func (r *RepositoryCache[T]) Count(ctx context.Context) (int64, error) {
	return r.base.Count(ctx)
}

// Close closes the underlying repository.
func (r *RepositoryCache[T]) Close() error {
	return r.base.Close()
}
