package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// MemoryCache implements the Cache interface using an in-memory map.
// This is a simple implementation that doesn't require external dependencies.
type MemoryCache struct {
	items map[string]cacheItem
	mu    sync.RWMutex
}

type cacheItem struct {
	value      []byte
	expiration time.Time
}

// NewMemoryCache creates a new in-memory cache implementation.
func NewMemoryCache(config CacheConfig) *MemoryCache {
	if !config.Enabled {
		return nil
	}

	c := &MemoryCache{
		items: make(map[string]cacheItem),
	}

	// Start cleanup goroutine
	go c.cleanup(config.CleanupInterval)

	return c
}

// cleanup periodically removes expired entries.
func (m *MemoryCache) cleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.Lock()
		now := time.Now()
		for key, item := range m.items {
			if now.After(item.expiration) {
				delete(m.items, key)
			}
		}
		m.mu.Unlock()
	}
}

// Get retrieves a value from the cache.
func (m *MemoryCache) Get(ctx context.Context, key string, dest interface{}) (bool, error) {
	if m == nil {
		return false, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	item, found := m.items[key]
	if !found {
		return false, nil
	}

	// Check expiration
	if time.Now().After(item.expiration) {
		// Expired, remove it
		m.mu.RUnlock()
		m.mu.Lock()
		delete(m.items, key)
		m.mu.Unlock()
		m.mu.RLock()
		return false, nil
	}

	// Deserialize
	if err := json.Unmarshal(item.value, dest); err != nil {
		return false, fmt.Errorf("failed to unmarshal cached value: %w", err)
	}

	return true, nil
}

// Set stores a value in the cache with the specified TTL.
func (m *MemoryCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if m == nil {
		return nil // Cache disabled, silently succeed
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Serialize the value
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value for cache: %w", err)
	}

	m.items[key] = cacheItem{
		value:      data,
		expiration: time.Now().Add(ttl),
	}

	return nil
}

// Delete removes a value from the cache.
func (m *MemoryCache) Delete(ctx context.Context, key string) error {
	if m == nil {
		return nil // Cache disabled, silently succeed
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.items, key)
	return nil
}

// Flush removes all entries from the cache.
func (m *MemoryCache) Flush(ctx context.Context) error {
	if m == nil {
		return nil // Cache disabled, silently succeed
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.items = make(map[string]cacheItem)
	return nil
}
