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

	cacheConfig := cache.CacheConfig{
		Enabled:         true,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
	}

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
