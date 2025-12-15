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

// =============================================================================
// Builder Pattern Tests
// =============================================================================

func TestDatabaseBuilder_Build_WithoutCache(t *testing.T) {
	// Arrange
	ctx := context.Background()
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	// Act
	db, err := NewDatabaseBuilder[BuilderTestEntity](ctx, "test_no_cache").
		Build()

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, db)

	t.Cleanup(func() {
		_ = db.Close()
	})
}

func TestDatabaseBuilder_Build_WithCache(t *testing.T) {
	// Arrange
	ctx := context.Background()
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	cacheConfig := cache.CacheConfig{
		Enabled:         true,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
	}

	// Act
	db, err := NewDatabaseBuilder[BuilderTestEntity](ctx, "test_with_cache").
		WithCache(cacheConfig).
		Build()

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, db)

	t.Cleanup(func() {
		_ = db.Close()
	})
}

func TestDatabaseBuilder_Build_CacheDisabledInConfig(t *testing.T) {
	// Arrange
	ctx := context.Background()
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	cacheConfig := cache.CacheConfig{
		Enabled: false, // Explicitly disabled
	}

	// Act
	db, err := NewDatabaseBuilder[BuilderTestEntity](ctx, "test_cache_disabled").
		WithCache(cacheConfig).
		Build()

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, db)

	// Verify it behaves like non-cached repository
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
	// Arrange
	ctx := context.Background()
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	cacheConfig := cache.CacheConfig{
		Enabled:         true,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
	}

	// Act - Builder pattern allows method chaining
	builder := NewDatabaseBuilder[BuilderTestEntity](ctx, "test_chaining")
	builder = builder.WithCache(cacheConfig)
	db, err := builder.Build()

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, db)

	t.Cleanup(func() {
		_ = db.Close()
	})
}

func TestDatabaseBuilder_Build_ReturnsRepositoryInterface(t *testing.T) {
	// Arrange
	ctx := context.Background()
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	// Act
	db, err := NewDatabaseBuilder[BuilderTestEntity](ctx, "test_interface").Build()

	// Assert
	require.NoError(t, err)

	// Verify it implements the RepositoryInterface
	var _ interfaces.RepositoryInterface[BuilderTestEntity] = db

	t.Cleanup(func() {
		_ = db.Close()
	})
}

// =============================================================================
// Backwards Compatibility Tests
// =============================================================================

func TestNewDatabase_StillWorks(t *testing.T) {
	// Arrange
	ctx := context.Background()
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	// Act - Original function should still work
	db, err := NewDatabase[BuilderTestEntity](ctx, "test_backwards_compat")

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, db)

	t.Cleanup(func() {
		_ = db.Close()
	})
}

func TestDatabaseBuilder_MultipleBuilds(t *testing.T) {
	// Arrange
	ctx := context.Background()
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	builder := NewDatabaseBuilder[BuilderTestEntity](ctx, "test_multiple")

	// Act - Build multiple times
	db1, err1 := builder.Build()
	db2, err2 := builder.Build()

	// Assert
	require.NoError(t, err1)
	require.NoError(t, err2)
	assert.NotNil(t, db1)
	assert.NotNil(t, db2)

	// They should be different instances
	assert.NotEqual(t, db1, db2)

	t.Cleanup(func() {
		_ = db1.Close()
		_ = db2.Close()
	})
}
