package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/core/database/cache"
)

type IntegrationTestEntity struct {
	ID   uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	Name string    `gorm:"not null" json:"name"`
}

func (IntegrationTestEntity) TableName() string {
	return "integration_test_entities"
}

// Integration test with real database and cache
func TestDatabaseWithCache_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	tempDir := t.TempDir()
	os.Setenv("QUIVER_DATABASE_PATH", tempDir)
	defer os.Unsetenv("QUIVER_DATABASE_PATH")

	cacheConfig := cache.DefaultCacheConfig()

	// Create cached database
	db, err := NewDatabaseBuilder[IntegrationTestEntity](ctx, "integration_test").
		WithCache(cacheConfig).
		Build()
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = db.Close()
	})

	// Test CRUD with caching
	entity := &IntegrationTestEntity{
		ID:   uuid.New(),
		Name: "Integration Test Entity",
	}

	// Create
	created, err := db.Create(ctx, entity)
	require.NoError(t, err)
	assert.Equal(t, entity.Name, created.Name)

	// Read (should be cached after first read)
	read1, err := db.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.Name, read1.Name)

	read2, err := db.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.Name, read2.Name)

	// Update (should invalidate cache)
	created.Name = "Updated Name"
	updated, err := db.Update(ctx, created)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", updated.Name)

	// Read after update (should fetch fresh data)
	readAfterUpdate, err := db.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", readAfterUpdate.Name)

	// Delete
	err = db.Delete(ctx, created.ID)
	require.NoError(t, err)

	// Read after delete (should fail)
	_, err = db.GetByID(ctx, created.ID)
	assert.Error(t, err)
}

func TestCacheVsNonCache_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	tempDir := t.TempDir()
	os.Setenv("QUIVER_DATABASE_PATH", tempDir)
	defer os.Unsetenv("QUIVER_DATABASE_PATH")

	cacheConfig := cache.DefaultCacheConfig()

	// Create both cached and non-cached databases using the same underlying database
	dbName := "cache_comparison_test"

	// Cached database
	cachedDB, err := NewDatabaseBuilder[IntegrationTestEntity](ctx, dbName).
		WithCache(cacheConfig).
		Build()
	require.NoError(t, err)
	defer cachedDB.Close()

	// Non-cached database (pointing to same database file)
	nonCachedDB, err := NewDatabaseBuilder[IntegrationTestEntity](ctx, dbName).
		Build()
	require.NoError(t, err)
	defer nonCachedDB.Close()

	// Test 1: Create operation - both should work identically
	entity := &IntegrationTestEntity{
		ID:   uuid.New(),
		Name: "Comparison Test Entity",
	}

	cachedCreated, err := cachedDB.Create(ctx, entity)
	require.NoError(t, err)
	assert.Equal(t, entity.Name, cachedCreated.Name)

	// Non-cached should see the same data (same underlying DB)
	nonCachedRead, err := nonCachedDB.GetByID(ctx, cachedCreated.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.Name, nonCachedRead.Name)

	// Test 2: Read operation - cached should return same data
	cachedRead1, err := cachedDB.GetByID(ctx, cachedCreated.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.Name, cachedRead1.Name)

	cachedRead2, err := cachedDB.GetByID(ctx, cachedCreated.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.Name, cachedRead2.Name)
	// Both reads should return identical data (second read from cache)

	// Test 3: Update via non-cached - cached instance still has stale data (expected behavior)
	// This is a fundamental caching tradeoff: updates made outside a cached instance
	// won't be visible until the cache expires or is explicitly invalidated.
	nonCachedRead.Name = "Updated via Non-Cached"
	nonCachedUpdated, err := nonCachedDB.Update(ctx, nonCachedRead)
	require.NoError(t, err)
	assert.Equal(t, "Updated via Non-Cached", nonCachedUpdated.Name)

	// Cached DB still returns stale data (cache hit with old value)
	// This is expected behavior - the cached instance has no way to know about
	// updates made by the non-cached instance
	cachedReadAfterUpdate, err := cachedDB.GetByID(ctx, cachedCreated.ID)
	require.NoError(t, err)
	assert.Equal(t, "Comparison Test Entity", cachedReadAfterUpdate.Name) // Still stale

	// Test 4: Update via cached - should invalidate cache correctly
	// First, let's get the actual current state from the database via the cached instance
	// (this will return cached/stale data, but we'll update it anyway to test invalidation)
	cachedReadAfterUpdate.Name = "Updated via Cached"
	cachedUpdated, err := cachedDB.Update(ctx, cachedReadAfterUpdate)
	require.NoError(t, err)
	assert.Equal(t, "Updated via Cached", cachedUpdated.Name)

	// Next read from cached DB should get fresh data
	cachedReadAfterCachedUpdate, err := cachedDB.GetByID(ctx, cachedCreated.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated via Cached", cachedReadAfterCachedUpdate.Name)

	// Non-cached should also see the update
	nonCachedReadAfterCachedUpdate, err := nonCachedDB.GetByID(ctx, cachedCreated.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated via Cached", nonCachedReadAfterCachedUpdate.Name)

	// Test 5: List operations - both should return same data
	cachedList, err := cachedDB.Get(ctx)
	require.NoError(t, err)
	assert.Len(t, cachedList, 1)

	nonCachedList, err := nonCachedDB.Get(ctx)
	require.NoError(t, err)
	assert.Len(t, nonCachedList, 1)

	// Test 6: Delete operation - both should reflect deletion
	err = cachedDB.Delete(ctx, cachedCreated.ID)
	require.NoError(t, err)

	// Both should fail to find deleted entity
	_, err = cachedDB.GetByID(ctx, cachedCreated.ID)
	assert.Error(t, err)

	_, err = nonCachedDB.GetByID(ctx, cachedCreated.ID)
	assert.Error(t, err)

	// Both should return empty lists
	cachedListAfterDelete, err := cachedDB.Get(ctx)
	require.NoError(t, err)
	assert.Len(t, cachedListAfterDelete, 0)

	nonCachedListAfterDelete, err := nonCachedDB.Get(ctx)
	require.NoError(t, err)
	assert.Len(t, nonCachedListAfterDelete, 0)
}

func TestDatabaseWithoutCache_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	tempDir := t.TempDir()
	os.Setenv("QUIVER_DATABASE_PATH", tempDir)
	defer os.Unsetenv("QUIVER_DATABASE_PATH")

	// Create database without cache
	db, err := NewDatabaseBuilder[IntegrationTestEntity](ctx, "no_cache_test").
		Build()
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = db.Close()
	})

	// Verify basic operations work without cache
	entity := &IntegrationTestEntity{
		ID:   uuid.New(),
		Name: "No Cache Entity",
	}

	created, err := db.Create(ctx, entity)
	require.NoError(t, err)

	read, err := db.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.Name, read.Name)
}

func TestDatabaseWithCache_GetListCaching(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	tempDir := t.TempDir()
	os.Setenv("QUIVER_DATABASE_PATH", tempDir)
	defer os.Unsetenv("QUIVER_DATABASE_PATH")

	cacheConfig := cache.CacheConfig{
		Enabled:         true,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
	}

	db, err := NewDatabaseBuilder[IntegrationTestEntity](ctx, "list_cache_test").
		WithCache(cacheConfig).
		Build()
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = db.Close()
	})

	// Create multiple entities
	entities := []*IntegrationTestEntity{
		{ID: uuid.New(), Name: "Entity 1"},
		{ID: uuid.New(), Name: "Entity 2"},
		{ID: uuid.New(), Name: "Entity 3"},
	}

	for _, entity := range entities {
		_, err := db.Create(ctx, entity)
		require.NoError(t, err)
	}

	// First Get should populate cache
	first, err := db.Get(ctx)
	require.NoError(t, err)
	assert.Len(t, first, 3)

	// Second Get should use cache (we can't verify this directly, but it should work)
	second, err := db.Get(ctx)
	require.NoError(t, err)
	assert.Len(t, second, 3)

	// Create new entity should invalidate cache
	newEntity := &IntegrationTestEntity{ID: uuid.New(), Name: "Entity 4"}
	_, err = db.Create(ctx, newEntity)
	require.NoError(t, err)

	// Get should now return 4 entities (cache invalidated)
	third, err := db.Get(ctx)
	require.NoError(t, err)
	assert.Len(t, third, 4)
}
