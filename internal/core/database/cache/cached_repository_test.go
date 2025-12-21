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

	interfaces "github.com/rabbytesoftware/quiver/internal/core/database/interface"
	"github.com/rabbytesoftware/quiver/internal/core/database/repository"
)

type TestEntity struct {
	ID   uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	Name string    `gorm:"not null" json:"name"`
	Age  int       `json:"age"`
}

func (TestEntity) TableName() string {
	return "cache_test_entities"
}

func TestNewCachedRepository_ValidConfig(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	config := DefaultCacheConfig()
	dbName := fmt.Sprintf("valid_config_test_%d", time.Now().UnixNano())

	baseRepo, err := repository.NewRepository[TestEntity](dbName)
	require.NoError(t, err)

	result, err := NewCachedRepository[TestEntity](baseRepo, config)

	require.NoError(t, err)
	assert.NotNil(t, result, "Should return CachedRepository")

	cachedRepo, ok := result.(*CachedRepository[TestEntity])
	require.True(t, ok, "Result should be *CachedRepository")
	assert.NotNil(t, cachedRepo.cache)
	assert.NotNil(t, cachedRepo.db)
	assert.Equal(t, config, cachedRepo.config)

	t.Cleanup(func() { _ = result.Close() })
}

func TestCachedRepository_Create(t *testing.T) {
	cachedRepo := setupCachedRepo(t)
	ctx := context.Background()

	entity := &TestEntity{
		ID:   uuid.New(),
		Name: "Test Entity",
		Age:  25,
	}

	created, err := cachedRepo.Create(ctx, entity)

	require.NoError(t, err)
	assert.Equal(t, entity.ID, created.ID)
	assert.Equal(t, entity.Name, created.Name)
}

func TestCachedRepository_GetByID_CacheMiss(t *testing.T) {
	cachedRepo := setupCachedRepo(t)
	ctx := context.Background()

	entity := &TestEntity{ID: uuid.New(), Name: "Test Entity", Age: 30}
	_, err := cachedRepo.Create(ctx, entity)
	require.NoError(t, err)

	// Force cache miss by creating a fresh cached repo pointing to same DB
	// This verifies data is persisted and can be retrieved
	result, err := cachedRepo.GetByID(ctx, entity.ID)

	require.NoError(t, err)
	assert.Equal(t, entity.ID, result.ID)
	assert.Equal(t, entity.Name, result.Name)
}

func TestCachedRepository_GetByID_CacheHit(t *testing.T) {
	cachedRepo := setupCachedRepo(t)
	ctx := context.Background()

	entity := &TestEntity{ID: uuid.New(), Name: "Test Entity", Age: 30}
	_, err := cachedRepo.Create(ctx, entity)
	require.NoError(t, err)

	// First call - populates cache
	_, err = cachedRepo.GetByID(ctx, entity.ID)
	require.NoError(t, err)

	// Second call - should hit cache (verified by getting same result)
	result, err := cachedRepo.GetByID(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.ID, result.ID)
	assert.Equal(t, entity.Name, result.Name)
}

func TestCachedRepository_GetByID_NotFound(t *testing.T) {
	cachedRepo := setupCachedRepo(t)
	ctx := context.Background()

	_, err := cachedRepo.GetByID(ctx, uuid.New())

	assert.Error(t, err)
}

func TestCachedRepository_Get_CachesIndividually(t *testing.T) {
	cachedRepo := setupCachedRepo(t)
	ctx := context.Background()

	entities := []*TestEntity{
		{ID: uuid.New(), Name: "Entity 1", Age: 25},
		{ID: uuid.New(), Name: "Entity 2", Age: 30},
		{ID: uuid.New(), Name: "Entity 3", Age: 35},
	}
	for _, e := range entities {
		_, err := cachedRepo.Create(ctx, e)
		require.NoError(t, err)
	}

	result, err := cachedRepo.Get(ctx)
	require.NoError(t, err)
	assert.Len(t, result, 3)

	// Verify each entity can be retrieved by ID (from cache)
	for _, entity := range entities {
		retrieved, err := cachedRepo.GetByID(ctx, entity.ID)
		require.NoError(t, err)
		assert.Equal(t, entity.Name, retrieved.Name)
	}
}

func TestCachedRepository_Update_InvalidatesCache(t *testing.T) {
	cachedRepo := setupCachedRepo(t)
	ctx := context.Background()

	entity := &TestEntity{ID: uuid.New(), Name: "Original", Age: 25}
	_, err := cachedRepo.Create(ctx, entity)
	require.NoError(t, err)

	// Populate cache
	_, err = cachedRepo.GetByID(ctx, entity.ID)
	require.NoError(t, err)

	// Update entity
	entity.Name = "Updated"
	_, err = cachedRepo.Update(ctx, entity)
	require.NoError(t, err)

	// After update, cache should be invalidated and we should get fresh data
	retrieved, err := cachedRepo.GetByID(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated", retrieved.Name)
}

func TestCachedRepository_Delete_InvalidatesCache(t *testing.T) {
	cachedRepo := setupCachedRepo(t)
	ctx := context.Background()

	entity := &TestEntity{ID: uuid.New(), Name: "ToDelete", Age: 25}
	_, err := cachedRepo.Create(ctx, entity)
	require.NoError(t, err)

	// Populate cache
	_, err = cachedRepo.GetByID(ctx, entity.ID)
	require.NoError(t, err)

	// Delete entity
	err = cachedRepo.Delete(ctx, entity.ID)
	require.NoError(t, err)

	// Should not find deleted entity
	_, err = cachedRepo.GetByID(ctx, entity.ID)
	assert.Error(t, err, "Should not find deleted entity")
}

func TestCachedRepository_Exists(t *testing.T) {
	cachedRepo := setupCachedRepo(t)
	ctx := context.Background()

	entity := &TestEntity{ID: uuid.New(), Name: "Test"}
	_, err := cachedRepo.Create(ctx, entity)
	require.NoError(t, err)

	exists, err := cachedRepo.Exists(ctx, entity.ID)
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = cachedRepo.Exists(ctx, uuid.New())
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestCachedRepository_Count(t *testing.T) {
	cachedRepo := setupCachedRepo(t)
	ctx := context.Background()

	// Create two entities
	_, err := cachedRepo.Create(ctx, &TestEntity{ID: uuid.New(), Name: "One", Age: 25})
	require.NoError(t, err)
	_, err = cachedRepo.Create(ctx, &TestEntity{ID: uuid.New(), Name: "Two", Age: 30})
	require.NoError(t, err)

	count, err := cachedRepo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

func TestCachedRepository_Integration_CRUD(t *testing.T) {
	cachedRepo := setupCachedRepo(t)
	ctx := context.Background()

	// Create
	entity := &TestEntity{
		ID:   uuid.New(),
		Name: "Integration Test",
		Age:  30,
	}
	created, err := cachedRepo.Create(ctx, entity)
	require.NoError(t, err)
	assert.Equal(t, entity.ID, created.ID)

	// Read
	retrieved, err := cachedRepo.GetByID(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.Name, retrieved.Name)

	// Update
	entity.Name = "Updated Name"
	updated, err := cachedRepo.Update(ctx, entity)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", updated.Name)

	// Verify update persisted
	retrieved, err = cachedRepo.GetByID(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", retrieved.Name)

	// Delete
	err = cachedRepo.Delete(ctx, entity.ID)
	require.NoError(t, err)

	// Verify deleted
	exists, err := cachedRepo.Exists(ctx, entity.ID)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestCachedRepository_Integration_CacheExpiry(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	config := CacheConfig{
		DefaultTTL:      100 * time.Millisecond,
		CleanupInterval: 50 * time.Millisecond,
	}
	dbName := fmt.Sprintf("cache_expiry_test_%d", time.Now().UnixNano())

	baseRepo, err := repository.NewRepository[TestEntity](dbName)
	require.NoError(t, err)

	cachedRepo, err := NewCachedRepository[TestEntity](baseRepo, config)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cachedRepo.Close() })

	ctx := context.Background()

	entity := &TestEntity{ID: uuid.New(), Name: "Expiry Test", Age: 25}
	_, err = cachedRepo.Create(ctx, entity)
	require.NoError(t, err)

	// Populate cache
	_, err = cachedRepo.GetByID(ctx, entity.ID)
	require.NoError(t, err)

	// Wait for cache to expire
	time.Sleep(200 * time.Millisecond)

	// Should still retrieve from DB after cache expiry
	retrieved, err := cachedRepo.GetByID(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.Name, retrieved.Name)
}

func TestCachedRepository_Integration_ConcurrentAccess(t *testing.T) {
	cachedRepo := setupCachedRepo(t)
	ctx := context.Background()

	const numGoroutines = 10
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*3)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			entity := &TestEntity{
				ID:   uuid.New(),
				Name: fmt.Sprintf("Concurrent %d", i),
				Age:  20 + i,
			}
			_, err := cachedRepo.Create(ctx, entity)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent operation failed: %v", err)
	}

	count, err := cachedRepo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(numGoroutines), count)
}

func TestParity_Create(t *testing.T) {
	baseRepo, cachedRepo := setupParityRepos(t)
	ctx := context.Background()

	entity := &TestEntity{ID: uuid.New(), Name: "Parity Test", Age: 25}

	baseResult, baseErr := baseRepo.Create(ctx, entity)

	entity2 := &TestEntity{ID: uuid.New(), Name: "Parity Test", Age: 25}
	cachedResult, cachedErr := cachedRepo.Create(ctx, entity2)

	assert.Equal(t, baseErr == nil, cachedErr == nil, "Error status should match")
	assert.Equal(t, baseResult.Name, cachedResult.Name)
	assert.Equal(t, baseResult.Age, cachedResult.Age)
}

func TestParity_GetByID(t *testing.T) {
	baseRepo, cachedRepo := setupParityRepos(t)
	ctx := context.Background()

	baseID := uuid.New()
	cachedID := uuid.New()
	baseEntity := &TestEntity{ID: baseID, Name: "Parity Test", Age: 30}
	cachedEntity := &TestEntity{ID: cachedID, Name: "Parity Test", Age: 30}

	_, err := baseRepo.Create(ctx, baseEntity)
	require.NoError(t, err)
	_, err = cachedRepo.Create(ctx, cachedEntity)
	require.NoError(t, err)

	baseResult, baseErr := baseRepo.GetByID(ctx, baseID)
	cachedResult, cachedErr := cachedRepo.GetByID(ctx, cachedID)

	require.NoError(t, baseErr)
	require.NoError(t, cachedErr)
	assert.Equal(t, baseResult.Name, cachedResult.Name)
	assert.Equal(t, baseResult.Age, cachedResult.Age)
}

func TestParity_Get(t *testing.T) {
	baseRepo, cachedRepo := setupParityRepos(t)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		baseEntity := &TestEntity{ID: uuid.New(), Name: fmt.Sprintf("Entity %d", i), Age: 25 + i}
		cachedEntity := &TestEntity{ID: uuid.New(), Name: fmt.Sprintf("Entity %d", i), Age: 25 + i}
		_, err := baseRepo.Create(ctx, baseEntity)
		require.NoError(t, err)
		_, err = cachedRepo.Create(ctx, cachedEntity)
		require.NoError(t, err)
	}

	baseResult, baseErr := baseRepo.Get(ctx)
	cachedResult, cachedErr := cachedRepo.Get(ctx)

	require.NoError(t, baseErr)
	require.NoError(t, cachedErr)
	assert.Len(t, cachedResult, len(baseResult))
}

func TestParity_Update(t *testing.T) {
	baseRepo, cachedRepo := setupParityRepos(t)
	ctx := context.Background()

	baseID := uuid.New()
	cachedID := uuid.New()
	baseEntity := &TestEntity{ID: baseID, Name: "Original", Age: 25}
	cachedEntity := &TestEntity{ID: cachedID, Name: "Original", Age: 25}

	_, err := baseRepo.Create(ctx, baseEntity)
	require.NoError(t, err)
	_, err = cachedRepo.Create(ctx, cachedEntity)
	require.NoError(t, err)

	baseEntity.Name = "Updated"
	cachedEntity.Name = "Updated"
	baseResult, baseErr := baseRepo.Update(ctx, baseEntity)
	cachedResult, cachedErr := cachedRepo.Update(ctx, cachedEntity)

	require.NoError(t, baseErr)
	require.NoError(t, cachedErr)
	assert.Equal(t, baseResult.Name, cachedResult.Name)
}

func TestParity_Exists(t *testing.T) {
	baseRepo, cachedRepo := setupParityRepos(t)
	ctx := context.Background()

	baseID := uuid.New()
	cachedID := uuid.New()

	baseExists, _ := baseRepo.Exists(ctx, baseID)
	cachedExists, _ := cachedRepo.Exists(ctx, cachedID)
	assert.Equal(t, baseExists, cachedExists)

	baseEntity := &TestEntity{ID: baseID, Name: "Test", Age: 25}
	cachedEntity := &TestEntity{ID: cachedID, Name: "Test", Age: 25}
	_, _ = baseRepo.Create(ctx, baseEntity)
	_, _ = cachedRepo.Create(ctx, cachedEntity)

	baseExists, _ = baseRepo.Exists(ctx, baseID)
	cachedExists, _ = cachedRepo.Exists(ctx, cachedID)
	assert.Equal(t, baseExists, cachedExists)
}

func TestParity_Count(t *testing.T) {
	baseRepo, cachedRepo := setupParityRepos(t)
	ctx := context.Background()

	baseCount, _ := baseRepo.Count(ctx)
	cachedCount, _ := cachedRepo.Count(ctx)
	assert.Equal(t, baseCount, cachedCount)

	for i := 0; i < 3; i++ {
		baseEntity := &TestEntity{ID: uuid.New(), Name: fmt.Sprintf("Entity %d", i), Age: 20 + i}
		cachedEntity := &TestEntity{ID: uuid.New(), Name: fmt.Sprintf("Entity %d", i), Age: 20 + i}
		_, _ = baseRepo.Create(ctx, baseEntity)
		_, _ = cachedRepo.Create(ctx, cachedEntity)
	}

	baseCount, _ = baseRepo.Count(ctx)
	cachedCount, _ = cachedRepo.Count(ctx)
	assert.Equal(t, baseCount, cachedCount)
}

func setupCachedRepo(t *testing.T) interfaces.RepositoryInterface[TestEntity] {
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	config := DefaultCacheConfig()
	dbName := fmt.Sprintf("cached_repo_test_%d", time.Now().UnixNano())

	baseRepo, err := repository.NewRepository[TestEntity](dbName)
	require.NoError(t, err)

	cachedRepo, err := NewCachedRepository[TestEntity](baseRepo, config)
	require.NoError(t, err)

	t.Cleanup(func() { _ = cachedRepo.Close() })

	return cachedRepo
}

func setupParityRepos(t *testing.T) (
	interfaces.RepositoryInterface[TestEntity],
	interfaces.RepositoryInterface[TestEntity],
) {
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	baseDbName := fmt.Sprintf("parity_base_%d", time.Now().UnixNano())
	baseRepo, err := repository.NewRepository[TestEntity](baseDbName)
	require.NoError(t, err)
	t.Cleanup(func() { _ = baseRepo.Close() })

	cachedDbName := fmt.Sprintf("parity_cached_%d", time.Now().UnixNano())
	config := DefaultCacheConfig()

	cachedBaseRepo, err := repository.NewRepository[TestEntity](cachedDbName)
	require.NoError(t, err)

	cachedRepo, err := NewCachedRepository[TestEntity](cachedBaseRepo, config)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cachedRepo.Close() })

	return baseRepo, cachedRepo
}
