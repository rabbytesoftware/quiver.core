package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/patrickmn/go-cache"
)

// GoCache implements the Cache interface using github.com/patrickmn/go-cache.
type GoCache struct {
	cache *cache.Cache
}

// NewGoCache creates a new go-cache implementation.
func NewGoCache(config CacheConfig) *GoCache {
	if !config.Enabled {
		return nil
	}

	return &GoCache{
		cache: cache.New(config.DefaultTTL, config.CleanupInterval),
	}
}

// Get retrieves a value from the cache.
func (g *GoCache) Get(ctx context.Context, key string, dest interface{}) (bool, error) {
	if g == nil || g.cache == nil {
		return false, nil
	}

	value, found := g.cache.Get(key)
	if !found {
		return false, nil
	}

	// go-cache stores values as interface{}, so we need to handle serialization
	// If the value is already []byte (from our Set), use it directly
	data, ok := value.([]byte)
	if !ok {
		// If not bytes, serialize it
		var err error
		data, err = json.Marshal(value)
		if err != nil {
			return false, fmt.Errorf("failed to marshal cached value: %w", err)
		}
	}

	// Deserialize into destination
	if err := json.Unmarshal(data, dest); err != nil {
		return false, fmt.Errorf("failed to unmarshal cached value: %w", err)
	}

	return true, nil
}

// Set stores a value in the cache with the specified TTL.
func (g *GoCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if g == nil || g.cache == nil {
		return nil // Cache disabled, silently succeed
	}

	// Serialize the value to ensure type safety and consistency
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value for cache: %w", err)
	}

	// Store as []byte for consistent retrieval
	g.cache.Set(key, data, ttl)
	return nil
}

// Delete removes a value from the cache.
func (g *GoCache) Delete(ctx context.Context, key string) error {
	if g == nil || g.cache == nil {
		return nil // Cache disabled, silently succeed
	}

	g.cache.Delete(key)
	return nil
}

// Flush removes all entries from the cache.
func (g *GoCache) Flush(ctx context.Context) error {
	if g == nil || g.cache == nil {
		return nil // Cache disabled, silently succeed
	}

	g.cache.Flush()
	return nil
}

