package cache

import (
	"context"
	"time"
)

// Cache defines the interface for caching operations.
// Implementations should be thread-safe and support TTL-based expiration.
type Cache interface {
	// Get retrieves a value from the cache.
	// Returns (found, error) where found indicates if the key exists.
	Get(ctx context.Context, key string, dest interface{}) (bool, error)

	// Set stores a value in the cache with the specified TTL.
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error

	// Delete removes a value from the cache.
	Delete(ctx context.Context, key string) error

	// Flush removes all entries from the cache.
	Flush(ctx context.Context) error
}

