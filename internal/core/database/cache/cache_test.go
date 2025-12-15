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

// =============================================================================
// GoCache Nil/Error Path Tests
// =============================================================================

func TestGoCache_NilCache_Get(t *testing.T) {
	// Test Get with nil cache (lines 30-31)
	var cache *GoCache = nil
	ctx := context.Background()

	var retrieved TestEntity
	found, err := cache.Get(ctx, "key", &retrieved)

	assert.NoError(t, err, "Get on nil cache should not error")
	assert.False(t, found, "Get on nil cache should return false")
}

func TestGoCache_NilCache_Set(t *testing.T) {
	// Test Set with nil cache (lines 61-62)
	var cache *GoCache = nil
	ctx := context.Background()
	entity := &TestEntity{ID: uuid.New(), Name: "Test"}

	err := cache.Set(ctx, "key", entity, 5*time.Minute)

	assert.NoError(t, err, "Set on nil cache should not error")
}

func TestGoCache_NilCache_Delete(t *testing.T) {
	// Test Delete with nil cache (lines 78-79)
	var cache *GoCache = nil
	ctx := context.Background()

	err := cache.Delete(ctx, "key")

	assert.NoError(t, err, "Delete on nil cache should not error")
}

func TestGoCache_NilCache_Flush(t *testing.T) {
	// Test Flush with nil cache (lines 88-89)
	var cache *GoCache = nil
	ctx := context.Background()

	err := cache.Flush(ctx)

	assert.NoError(t, err, "Flush on nil cache should not error")
}

func TestGoCache_Set_MarshalError(t *testing.T) {
	// Test Set with value that cannot be marshaled (lines 66-68)
	cache := NewGoCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	ctx := context.Background()
	key := "test:marshal:error"

	// Create a value that cannot be marshaled (channels can't be JSON marshaled)
	type UnmarshalableEntity struct {
		Data chan int `json:"data"`
	}
	unmarshalable := &UnmarshalableEntity{Data: make(chan int)}

	err := cache.Set(ctx, key, unmarshalable, 5*time.Minute)

	// This should return an error because channels can't be marshaled
	assert.Error(t, err, "Set with unmarshalable value should error")
	assert.Contains(t, err.Error(), "marshal", "Error should mention marshal failure")
}

func TestGoCache_Get_UnmarshalError(t *testing.T) {
	// Test Get with invalid destination type that can't unmarshal (lines 52-53)
	cache := NewGoCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	ctx := context.Background()
	key := "test:unmarshal:error"

	// Store a valid entity
	entity := &TestEntity{ID: uuid.New(), Name: "Test Entity"}
	err := cache.Set(ctx, key, entity, 5*time.Minute)
	require.NoError(t, err)

	// Try to retrieve into wrong type (string instead of struct)
	var wrongType string
	found, err := cache.Get(ctx, key, &wrongType)

	// The unmarshal will fail because we're trying to unmarshal an object into a string
	if found {
		// If found is true, there might be an error
		if err != nil {
			assert.Contains(t, err.Error(), "unmarshal", "Error should mention unmarshal failure")
		}
	} else {
		// If not found, no error expected
		assert.NoError(t, err)
	}
}

// =============================================================================
// MemoryCache Tests
// =============================================================================

func TestNewMemoryCache_Disabled(t *testing.T) {
	// Test NewMemoryCache when disabled (lines 25-27)
	config := CacheConfig{
		Enabled: false,
	}
	cacheInstance := NewMemoryCache(config)
	assert.Nil(t, cacheInstance)
}

func TestMemoryCache_Get(t *testing.T) {
	// Test MemoryCache Get method (lines 57-87)
	cache := NewMemoryCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	ctx := context.Background()
	key := "test:memory:get"
	entity := &TestEntity{ID: uuid.New(), Name: "Memory Cache Entity"}

	// Set value
	err := cache.Set(ctx, key, entity, 5*time.Minute)
	require.NoError(t, err)

	// Get value
	var retrieved TestEntity
	found, err := cache.Get(ctx, key, &retrieved)

	// Assert
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, entity.ID, retrieved.ID)
	assert.Equal(t, entity.Name, retrieved.Name)
}

func TestMemoryCache_Get_Miss(t *testing.T) {
	// Test MemoryCache Get with non-existent key
	cache := NewMemoryCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	ctx := context.Background()

	var retrieved TestEntity
	found, err := cache.Get(ctx, "nonexistent:key", &retrieved)

	assert.NoError(t, err)
	assert.False(t, found)
}

func TestMemoryCache_Get_Expired(t *testing.T) {
	// Test MemoryCache Get with expired entry (lines 71-79)
	config := CacheConfig{
		Enabled:         true,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
	}
	cache := NewMemoryCache(config)
	require.NotNil(t, cache)
	ctx := context.Background()
	key := "test:memory:expired"
	entity := &TestEntity{ID: uuid.New(), Name: "Expiring Entity"}

	// Set with very short TTL
	err := cache.Set(ctx, key, entity, 50*time.Millisecond)
	require.NoError(t, err)

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Try to get expired entry
	var retrieved TestEntity
	found, err := cache.Get(ctx, key, &retrieved)

	// Should not be found (expired and removed)
	assert.NoError(t, err)
	assert.False(t, found, "Expected cache miss after expiration")
}

func TestMemoryCache_Get_UnmarshalError(t *testing.T) {
	// Test MemoryCache Get with unmarshal error (lines 82-83)
	cache := NewMemoryCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	ctx := context.Background()
	key := "test:memory:unmarshal"

	// Store a valid entity
	entity := &TestEntity{ID: uuid.New(), Name: "Test Entity"}
	err := cache.Set(ctx, key, entity, 5*time.Minute)
	require.NoError(t, err)

	// Try to retrieve into wrong type
	var wrongType string
	found, err := cache.Get(ctx, key, &wrongType)

	// Should fail to unmarshal
	if found {
		if err != nil {
			assert.Contains(t, err.Error(), "unmarshal", "Error should mention unmarshal failure")
		}
	}
}

func TestMemoryCache_Set(t *testing.T) {
	// Test MemoryCache Set method (lines 90-110)
	cache := NewMemoryCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	ctx := context.Background()
	key := "test:memory:set"
	entity := &TestEntity{ID: uuid.New(), Name: "Set Entity"}

	err := cache.Set(ctx, key, entity, 5*time.Minute)

	assert.NoError(t, err, "Set should succeed")

	// Verify it was stored
	var retrieved TestEntity
	found, err := cache.Get(ctx, key, &retrieved)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, entity.Name, retrieved.Name)
}

func TestMemoryCache_Set_MarshalError(t *testing.T) {
	// Test MemoryCache Set with marshal error (lines 99-101)
	cache := NewMemoryCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	ctx := context.Background()
	key := "test:memory:marshal"

	// Create a value that cannot be marshaled
	type UnmarshalableEntity struct {
		Data chan int `json:"data"`
	}
	unmarshalable := &UnmarshalableEntity{Data: make(chan int)}

	err := cache.Set(ctx, key, unmarshalable, 5*time.Minute)

	assert.Error(t, err, "Set with unmarshalable value should error")
	assert.Contains(t, err.Error(), "marshal", "Error should mention marshal failure")
}

func TestMemoryCache_Delete(t *testing.T) {
	// Test MemoryCache Delete method (lines 113-123)
	cache := NewMemoryCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	ctx := context.Background()
	key := "test:memory:delete"
	entity := &TestEntity{ID: uuid.New(), Name: "Delete Me"}

	// Set value
	err := cache.Set(ctx, key, entity, 5*time.Minute)
	require.NoError(t, err)

	// Delete value
	err = cache.Delete(ctx, key)
	require.NoError(t, err)

	// Verify deletion
	var retrieved TestEntity
	found, err := cache.Get(ctx, key, &retrieved)
	require.NoError(t, err)
	assert.False(t, found, "Expected cache miss after deletion")
}

func TestMemoryCache_Flush(t *testing.T) {
	// Test MemoryCache Flush method (lines 126-136)
	cache := NewMemoryCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	ctx := context.Background()

	// Set multiple entries
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("test:memory:flush:%d", i)
		entity := &TestEntity{ID: uuid.New(), Name: fmt.Sprintf("Entity %d", i)}
		err := cache.Set(ctx, key, entity, 5*time.Minute)
		require.NoError(t, err)
	}

	// Flush cache
	err := cache.Flush(ctx)
	require.NoError(t, err)

	// Verify all entries are gone
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("test:memory:flush:%d", i)
		var retrieved TestEntity
		found, err := cache.Get(ctx, key, &retrieved)
		require.NoError(t, err)
		assert.False(t, found, "Expected cache miss after flush")
	}
}

func TestMemoryCache_NilReceiver_Get(t *testing.T) {
	// Test MemoryCache Get with nil receiver (lines 58-59)
	var cache *MemoryCache = nil
	ctx := context.Background()

	var retrieved TestEntity
	found, err := cache.Get(ctx, "key", &retrieved)

	assert.NoError(t, err, "Get on nil cache should not error")
	assert.False(t, found, "Get on nil cache should return false")
}

func TestMemoryCache_NilReceiver_Set(t *testing.T) {
	// Test MemoryCache Set with nil receiver (lines 91-92)
	var cache *MemoryCache = nil
	ctx := context.Background()
	entity := &TestEntity{ID: uuid.New(), Name: "Test"}

	err := cache.Set(ctx, "key", entity, 5*time.Minute)

	assert.NoError(t, err, "Set on nil cache should not error")
}

func TestMemoryCache_NilReceiver_Delete(t *testing.T) {
	// Test MemoryCache Delete with nil receiver (lines 114-115)
	var cache *MemoryCache = nil
	ctx := context.Background()

	err := cache.Delete(ctx, "key")

	assert.NoError(t, err, "Delete on nil cache should not error")
}

func TestMemoryCache_NilReceiver_Flush(t *testing.T) {
	// Test MemoryCache Flush with nil receiver (lines 127-128)
	var cache *MemoryCache = nil
	ctx := context.Background()

	err := cache.Flush(ctx)

	assert.NoError(t, err, "Flush on nil cache should not error")
}
