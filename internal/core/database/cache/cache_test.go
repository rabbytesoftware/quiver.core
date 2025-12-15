package cache

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEntity for cache testing
type TestEntity struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// =============================================================================
// Cache Interface Contract Tests
// =============================================================================

func TestCache_Set_And_Get(t *testing.T) {
	// Arrange
	cache := NewGoCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	ctx := context.Background()
	key := "test:entity:123"
	entity := &TestEntity{ID: uuid.New(), Name: "Test Entity"}

	// Act
	err := cache.Set(ctx, key, entity, 5*time.Minute)
	require.NoError(t, err)

	var retrieved TestEntity
	found, err := cache.Get(ctx, key, &retrieved)

	// Assert
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, entity.ID, retrieved.ID)
	assert.Equal(t, entity.Name, retrieved.Name)
}

func TestCache_Get_Miss(t *testing.T) {
	// Arrange
	cache := NewGoCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	ctx := context.Background()

	// Act
	var retrieved TestEntity
	found, err := cache.Get(ctx, "nonexistent:key", &retrieved)

	// Assert
	require.NoError(t, err)
	assert.False(t, found)
}

func TestCache_Get_TTLExpiry(t *testing.T) {
	// Arrange
	config := CacheConfig{
		Enabled:         true,
		DefaultTTL:      50 * time.Millisecond,
		CleanupInterval: 10 * time.Millisecond,
	}
	cache := NewGoCache(config)
	require.NotNil(t, cache)
	ctx := context.Background()
	key := "test:entity:expiring"
	entity := &TestEntity{ID: uuid.New(), Name: "Expiring Entity"}

	// Act
	err := cache.Set(ctx, key, entity, 50*time.Millisecond)
	require.NoError(t, err)

	// Wait for TTL to expire
	time.Sleep(100 * time.Millisecond)

	var retrieved TestEntity
	found, err := cache.Get(ctx, key, &retrieved)

	// Assert
	require.NoError(t, err)
	assert.False(t, found, "Expected cache miss after TTL expiry")
}

func TestCache_Delete(t *testing.T) {
	// Arrange
	cache := NewGoCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	ctx := context.Background()
	key := "test:entity:to-delete"
	entity := &TestEntity{ID: uuid.New(), Name: "Delete Me"}

	err := cache.Set(ctx, key, entity, 5*time.Minute)
	require.NoError(t, err)

	// Act
	err = cache.Delete(ctx, key)
	require.NoError(t, err)

	var retrieved TestEntity
	found, err := cache.Get(ctx, key, &retrieved)

	// Assert
	require.NoError(t, err)
	assert.False(t, found, "Expected cache miss after deletion")
}

func TestCache_Delete_NonExistent(t *testing.T) {
	// Arrange
	cache := NewGoCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	ctx := context.Background()

	// Act - Deleting non-existent key should not error
	err := cache.Delete(ctx, "nonexistent:key")

	// Assert
	require.NoError(t, err)
}

func TestCache_Flush(t *testing.T) {
	// Arrange
	cache := NewGoCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	ctx := context.Background()

	// Set multiple entries
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("test:entity:%d", i)
		entity := &TestEntity{ID: uuid.New(), Name: fmt.Sprintf("Entity %d", i)}
		err := cache.Set(ctx, key, entity, 5*time.Minute)
		require.NoError(t, err)
	}

	// Act
	err := cache.Flush(ctx)
	require.NoError(t, err)

	// Assert - All entries should be gone
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("test:entity:%d", i)
		var retrieved TestEntity
		found, err := cache.Get(ctx, key, &retrieved)
		require.NoError(t, err)
		assert.False(t, found, "Expected cache miss after flush")
	}
}

func TestCache_ContextCancellation(t *testing.T) {
	// Arrange
	cache := NewGoCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Act & Assert - Operations should handle cancelled context gracefully
	entity := &TestEntity{ID: uuid.New(), Name: "Test"}

	// Set might still work for in-memory cache, but should not panic
	_ = cache.Set(ctx, "key", entity, 5*time.Minute)

	var retrieved TestEntity
	_, _ = cache.Get(ctx, "key", &retrieved)
}

// =============================================================================
// Concurrency Tests
// =============================================================================

func TestCache_ConcurrentAccess(t *testing.T) {
	// Arrange
	cache := NewGoCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	ctx := context.Background()
	const numGoroutines = 100

	// Act - Concurrent writes and reads
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	for i := 0; i < numGoroutines; i++ {
		// Concurrent writes
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("test:concurrent:%d", i)
			entity := &TestEntity{ID: uuid.New(), Name: fmt.Sprintf("Entity %d", i)}
			_ = cache.Set(ctx, key, entity, 5*time.Minute)
		}(i)

		// Concurrent reads
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("test:concurrent:%d", i)
			var retrieved TestEntity
			_, _ = cache.Get(ctx, key, &retrieved)
		}(i)
	}

	wg.Wait()

	// Assert - No panics, data integrity maintained
	// Verify some entries exist
	var count int
	for i := 0; i < numGoroutines; i++ {
		key := fmt.Sprintf("test:concurrent:%d", i)
		var retrieved TestEntity
		found, _ := cache.Get(ctx, key, &retrieved)
		if found {
			count++
		}
	}
	assert.Greater(t, count, 0, "Expected at least some cached entries")
}

func TestCache_ConcurrentDeleteAndGet(t *testing.T) {
	// Arrange
	cache := NewGoCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	ctx := context.Background()

	// Pre-populate cache
	for i := 0; i < 50; i++ {
		key := fmt.Sprintf("test:race:%d", i)
		entity := &TestEntity{ID: uuid.New(), Name: fmt.Sprintf("Entity %d", i)}
		_ = cache.Set(ctx, key, entity, 5*time.Minute)
	}

	// Act - Race between deletes and gets
	var wg sync.WaitGroup
	wg.Add(100)

	for i := 0; i < 50; i++ {
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("test:race:%d", i)
			_ = cache.Delete(ctx, key)
		}(i)

		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("test:race:%d", i)
			var retrieved TestEntity
			_, _ = cache.Get(ctx, key, &retrieved)
		}(i)
	}

	wg.Wait()

	// Assert - No panics occurred
}

// =============================================================================
// Cache Config Tests
// =============================================================================

func TestDefaultCacheConfig(t *testing.T) {
	config := DefaultCacheConfig()
	assert.True(t, config.Enabled)
	assert.Equal(t, 5*time.Minute, config.DefaultTTL)
	assert.Equal(t, 1*time.Minute, config.CleanupInterval)
}

func TestCacheConfig_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		config CacheConfig
		want   bool
	}{
		{
			name: "valid enabled config",
			config: CacheConfig{
				Enabled:         true,
				DefaultTTL:      5 * time.Minute,
				CleanupInterval: 1 * time.Minute,
			},
			want: true,
		},
		{
			name: "valid disabled config",
			config: CacheConfig{
				Enabled: false,
			},
			want: true,
		},
		{
			name: "invalid enabled config with zero TTL",
			config: CacheConfig{
				Enabled:         true,
				DefaultTTL:      0,
				CleanupInterval: 1 * time.Minute,
			},
			want: false,
		},
		{
			name: "invalid enabled config with zero cleanup",
			config: CacheConfig{
				Enabled:         true,
				DefaultTTL:      5 * time.Minute,
				CleanupInterval: 0,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.config.IsValid())
		})
	}
}

func TestNewGoCache_Disabled(t *testing.T) {
	config := CacheConfig{
		Enabled: false,
	}
	cacheInstance := NewGoCache(config)
	assert.Nil(t, cacheInstance)
}

func TestNewMemoryCache_Enabled(t *testing.T) {
	config := CacheConfig{
		Enabled:         true,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
	}
	cacheInstance := NewMemoryCache(config)
	assert.NotNil(t, cacheInstance)
}
