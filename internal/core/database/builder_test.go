package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/core/database/cache"
	interfaces "github.com/rabbytesoftware/quiver/internal/core/database/interface"
)

type BuilderTestEntity struct {
	ID   uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	Name string    `gorm:"not null" json:"name"`
}

func (BuilderTestEntity) TableName() string {
	return "builder_test_entities"
}

// builderTestEntityExtractKey extracts the ID as a string for cache keys
func builderTestEntityExtractKey(entity *BuilderTestEntity) string {
	if entity == nil {
		return ""
	}
	return entity.ID.String()
}

func TestDatabaseBuilder_Build_WithoutCache(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	db, err := NewDatabaseBuilder[BuilderTestEntity](ctx, "test_no_cache").
		Build()

	require.NoError(t, err)
	assert.NotNil(t, db)

	t.Cleanup(func() {
		_ = db.Close()
	})
}

func TestDatabaseBuilder_Build_WithCache(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	cacheConfig := cache.CacheConfig{
		NumCounters:       1e7,
		MaxCost:           100,
		BufferItems:       64,
		DefaultTTL:        5 * time.Minute,
		DefaultCostOfItem: 1,
	}

	db, err := NewDatabaseBuilder[BuilderTestEntity](ctx, "test_with_cache").
		WithCache(cacheConfig, builderTestEntityExtractKey).
		Build()

	require.NoError(t, err)
	assert.NotNil(t, db)

	t.Cleanup(func() {
		_ = db.Close()
	})
}

func TestDatabaseBuilder_Build_WithDefaultCacheConfig(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	cacheConfig := cache.DefaultCacheConfig()

	db, err := NewDatabaseBuilder[BuilderTestEntity](ctx, "test_default_cache").
		WithCache(cacheConfig, builderTestEntityExtractKey).
		Build()

	require.NoError(t, err)
	assert.NotNil(t, db)

	entity := &BuilderTestEntity{ID: uuid.New(), Name: "Test"}
	created, err := db.Create(ctx, entity)
	require.NoError(t, err)

	retrieved, err := db.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.Name, retrieved.Name)

	t.Cleanup(func() {
		_ = db.Close()
	})
}

func TestDatabaseBuilder_Build_InvalidCacheConfig_FallsBackToNonCached(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	cacheConfig := cache.CacheConfig{
		NumCounters:       0, // Invalid: must be > 0
		MaxCost:           100,
		BufferItems:       64,
		DefaultTTL:        5 * time.Minute,
		DefaultCostOfItem: 1,
	}

	db, err := NewDatabaseBuilder[BuilderTestEntity](ctx, "test_invalid_cache").
		WithCache(cacheConfig, builderTestEntityExtractKey).
		Build()

	require.NoError(t, err)
	assert.NotNil(t, db)

	entity := &BuilderTestEntity{ID: uuid.New(), Name: "Test"}
	created, err := db.Create(ctx, entity)
	require.NoError(t, err)

	retrieved, err := db.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.Name, retrieved.Name)

	t.Cleanup(func() {
		_ = db.Close()
	})
}

func TestDatabaseBuilder_Chaining(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	cacheConfig := cache.CacheConfig{
		NumCounters:       1e7,
		MaxCost:           100,
		BufferItems:       64,
		DefaultTTL:        5 * time.Minute,
		DefaultCostOfItem: 1,
	}

	builder := NewDatabaseBuilder[BuilderTestEntity](ctx, "test_chaining")
	builder = builder.WithCache(cacheConfig, builderTestEntityExtractKey)
	db, err := builder.Build()

	require.NoError(t, err)
	assert.NotNil(t, db)

	t.Cleanup(func() {
		_ = db.Close()
	})
}

func TestDatabaseBuilder_Build_ReturnsRepositoryInterface(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	db, err := NewDatabaseBuilder[BuilderTestEntity](ctx, "test_interface").Build()

	require.NoError(t, err)

	var _ interfaces.RepositoryInterface[BuilderTestEntity] = db

	t.Cleanup(func() {
		_ = db.Close()
	})
}

func TestNewDatabase_StillWorks(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	db, err := NewDatabase[BuilderTestEntity](ctx, "test_backwards_compat")

	require.NoError(t, err)
	assert.NotNil(t, db)

	t.Cleanup(func() {
		_ = db.Close()
	})
}

func TestDatabaseBuilder_MultipleBuilds(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	builder := NewDatabaseBuilder[BuilderTestEntity](ctx, "test_multiple")

	db1, err1 := builder.Build()
	db2, err2 := builder.Build()

	require.NoError(t, err1)
	require.NoError(t, err2)
	assert.NotNil(t, db1)
	assert.NotNil(t, db2)

	assert.NotEqual(t, db1, db2)

	t.Cleanup(func() {
		_ = db1.Close()
		_ = db2.Close()
	})
}
