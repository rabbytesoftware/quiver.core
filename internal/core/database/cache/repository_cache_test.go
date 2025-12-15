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

// MockRepository implements RepositoryInterface for testing
type MockRepository[T any] struct {
	mu            sync.Mutex
	getFunc       func(ctx context.Context) ([]*T, error)
	getByIDFunc   func(ctx context.Context, id uuid.UUID) (*T, error)
	createFunc    func(ctx context.Context, entity *T) (*T, error)
	updateFunc    func(ctx context.Context, entity *T) (*T, error)
	deleteFunc    func(ctx context.Context, id uuid.UUID) error
	existsFunc    func(ctx context.Context, id uuid.UUID) (bool, error)
	countFunc     func(ctx context.Context) (int64, error)
	closeFunc     func() error
	callCounts    map[string]int
}

func NewMockRepository[T any]() *MockRepository[T] {
	return &MockRepository[T]{
		callCounts: make(map[string]int),
	}
}

func (m *MockRepository[T]) recordCall(method string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCounts[method]++
}

func (m *MockRepository[T]) GetCallCount(method string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCounts[method]
}

func (m *MockRepository[T]) Get(ctx context.Context) ([]*T, error) {
	m.recordCall("Get")
	if m.getFunc != nil {
		return m.getFunc(ctx)
	}
	return nil, nil
}

func (m *MockRepository[T]) GetByID(ctx context.Context, id uuid.UUID) (*T, error) {
	m.recordCall("GetByID")
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, fmt.Errorf("entity with id %s not found", id)
}

func (m *MockRepository[T]) Create(ctx context.Context, entity *T) (*T, error) {
	m.recordCall("Create")
	if m.createFunc != nil {
		return m.createFunc(ctx, entity)
	}
	return entity, nil
}

func (m *MockRepository[T]) Update(ctx context.Context, entity *T) (*T, error) {
	m.recordCall("Update")
	if m.updateFunc != nil {
		return m.updateFunc(ctx, entity)
	}
	return entity, nil
}

func (m *MockRepository[T]) Delete(ctx context.Context, id uuid.UUID) error {
	m.recordCall("Delete")
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return nil
}

func (m *MockRepository[T]) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	m.recordCall("Exists")
	if m.existsFunc != nil {
		return m.existsFunc(ctx, id)
	}
	return false, nil
}

func (m *MockRepository[T]) Count(ctx context.Context) (int64, error) {
	m.recordCall("Count")
	if m.countFunc != nil {
		return m.countFunc(ctx)
	}
	return 0, nil
}

func (m *MockRepository[T]) Close() error {
	m.recordCall("Close")
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

type CacheTestEntity struct {
	ID   uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	Name string    `gorm:"not null" json:"name"`
}

func (CacheTestEntity) TableName() string {
	return "cache_test_entities"
}

// =============================================================================
// Cache Hit/Miss Tests
// =============================================================================

func TestRepositoryCache_GetByID_CacheHit(t *testing.T) {
	// Arrange
	mockRepo := NewMockRepository[CacheTestEntity]()
	cache := NewGoCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	cachedRepo := NewRepositoryCache(mockRepo, cache, DefaultCacheConfig())
	ctx := context.Background()

	entityID := uuid.New()
	entity := &CacheTestEntity{ID: entityID, Name: "Cached Entity"}

	// First call - cache miss, should call underlying repo
	mockRepo.getByIDFunc = func(ctx context.Context, id uuid.UUID) (*CacheTestEntity, error) {
		if id == entityID {
			return entity, nil
		}
		return nil, fmt.Errorf("not found")
	}

	// Act - First call (populates cache)
	first, err := cachedRepo.GetByID(ctx, entityID)
	require.NoError(t, err)
	assert.Equal(t, entity.Name, first.Name)

	// Act - Second call (should hit cache, NOT call underlying repo)
	second, err := cachedRepo.GetByID(ctx, entityID)
	require.NoError(t, err)
	assert.Equal(t, entity.Name, second.Name)

	// Assert - Underlying repo should only be called once
	assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
}

func TestRepositoryCache_GetByID_CacheMiss(t *testing.T) {
	// Arrange
	mockRepo := NewMockRepository[CacheTestEntity]()
	cache := NewGoCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	cachedRepo := NewRepositoryCache(mockRepo, cache, DefaultCacheConfig())
	ctx := context.Background()

	entityID := uuid.New()
	entity := &CacheTestEntity{ID: entityID, Name: "From Database"}

	mockRepo.getByIDFunc = func(ctx context.Context, id uuid.UUID) (*CacheTestEntity, error) {
		if id == entityID {
			return entity, nil
		}
		return nil, fmt.Errorf("not found")
	}

	// Act
	result, err := cachedRepo.GetByID(ctx, entityID)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, entity.Name, result.Name)
	assert.Equal(t, 1, mockRepo.GetCallCount("GetByID"))
}

func TestRepositoryCache_GetByID_NotFound(t *testing.T) {
	// Arrange
	mockRepo := NewMockRepository[CacheTestEntity]()
	cache := NewGoCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	cachedRepo := NewRepositoryCache(mockRepo, cache, DefaultCacheConfig())
	ctx := context.Background()

	entityID := uuid.New()
	notFoundErr := fmt.Errorf("entity with id %s not found", entityID)

	mockRepo.getByIDFunc = func(ctx context.Context, id uuid.UUID) (*CacheTestEntity, error) {
		return nil, notFoundErr
	}

	// Act
	result, err := cachedRepo.GetByID(ctx, entityID)

	// Assert
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRepositoryCache_Get_CacheHit(t *testing.T) {
	// Arrange
	mockRepo := NewMockRepository[CacheTestEntity]()
	cache := NewGoCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	cachedRepo := NewRepositoryCache(mockRepo, cache, DefaultCacheConfig())
	ctx := context.Background()

	entities := []*CacheTestEntity{
		{ID: uuid.New(), Name: "Entity 1"},
		{ID: uuid.New(), Name: "Entity 2"},
	}

	mockRepo.getFunc = func(ctx context.Context) ([]*CacheTestEntity, error) {
		return entities, nil
	}

	// Act - First call (populates cache)
	first, err := cachedRepo.Get(ctx)
	require.NoError(t, err)
	assert.Len(t, first, 2)

	// Act - Second call (cache hit)
	second, err := cachedRepo.Get(ctx)
	require.NoError(t, err)
	assert.Len(t, second, 2)

	// Assert
	assert.Equal(t, 1, mockRepo.GetCallCount("Get"))
}

// =============================================================================
// Cache Invalidation Tests
// =============================================================================

func TestRepositoryCache_Create_InvalidatesGetCache(t *testing.T) {
	// Arrange
	mockRepo := NewMockRepository[CacheTestEntity]()
	cache := NewGoCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	cachedRepo := NewRepositoryCache(mockRepo, cache, DefaultCacheConfig())
	ctx := context.Background()

	initialEntities := []*CacheTestEntity{
		{ID: uuid.New(), Name: "Entity 1"},
	}
	newEntity := &CacheTestEntity{ID: uuid.New(), Name: "New Entity"}
	updatedEntities := append(initialEntities, newEntity)

	callCount := 0
	mockRepo.getFunc = func(ctx context.Context) ([]*CacheTestEntity, error) {
		callCount++
		if callCount == 1 {
			return initialEntities, nil
		}
		return updatedEntities, nil
	}
	mockRepo.createFunc = func(ctx context.Context, entity *CacheTestEntity) (*CacheTestEntity, error) {
		return newEntity, nil
	}

	// Populate cache
	_, _ = cachedRepo.Get(ctx)

	// Act - Create should invalidate Get cache
	_, err := cachedRepo.Create(ctx, newEntity)
	require.NoError(t, err)

	// This should hit database again, not cache
	result, err := cachedRepo.Get(ctx)
	require.NoError(t, err)
	assert.Len(t, result, 2)

	// Assert - Get called twice (once before, once after invalidation)
	assert.Equal(t, 2, mockRepo.GetCallCount("Get"))
}

func TestRepositoryCache_Update_InvalidatesEntityCache(t *testing.T) {
	// Arrange
	mockRepo := NewMockRepository[CacheTestEntity]()
	cache := NewGoCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	cachedRepo := NewRepositoryCache(mockRepo, cache, DefaultCacheConfig())
	ctx := context.Background()

	entityID := uuid.New()
	originalEntity := &CacheTestEntity{ID: entityID, Name: "Original"}
	updatedEntity := &CacheTestEntity{ID: entityID, Name: "Updated"}

	callCount := 0
	mockRepo.getByIDFunc = func(ctx context.Context, id uuid.UUID) (*CacheTestEntity, error) {
		callCount++
		if callCount == 1 {
			return originalEntity, nil
		}
		return updatedEntity, nil
	}
	mockRepo.updateFunc = func(ctx context.Context, entity *CacheTestEntity) (*CacheTestEntity, error) {
		return updatedEntity, nil
	}

	// Populate cache
	_, _ = cachedRepo.GetByID(ctx, entityID)

	// Act - Update should invalidate cache
	_, err := cachedRepo.Update(ctx, updatedEntity)
	require.NoError(t, err)

	// This should hit database again
	result, err := cachedRepo.GetByID(ctx, entityID)
	require.NoError(t, err)
	assert.Equal(t, "Updated", result.Name)

	// Assert
	assert.Equal(t, 2, mockRepo.GetCallCount("GetByID"))
}

func TestRepositoryCache_Delete_InvalidatesEntityCache(t *testing.T) {
	// Arrange
	mockRepo := NewMockRepository[CacheTestEntity]()
	cache := NewGoCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	cachedRepo := NewRepositoryCache(mockRepo, cache, DefaultCacheConfig())
	ctx := context.Background()

	entityID := uuid.New()
	entity := &CacheTestEntity{ID: entityID, Name: "To Delete"}
	notFoundErr := fmt.Errorf("entity with id %s not found", entityID)

	callCount := 0
	mockRepo.getByIDFunc = func(ctx context.Context, id uuid.UUID) (*CacheTestEntity, error) {
		callCount++
		if callCount == 1 {
			return entity, nil
		}
		return nil, notFoundErr
	}
	mockRepo.deleteFunc = func(ctx context.Context, id uuid.UUID) error {
		return nil
	}

	// Populate cache
	_, _ = cachedRepo.GetByID(ctx, entityID)

	// Act - Delete should invalidate cache
	err := cachedRepo.Delete(ctx, entityID)
	require.NoError(t, err)

	// This should hit database again and return not found
	_, err = cachedRepo.GetByID(ctx, entityID)
	assert.Error(t, err)

	// Assert
	assert.Equal(t, 2, mockRepo.GetCallCount("GetByID"))
}

func TestRepositoryCache_Delete_InvalidatesGetCache(t *testing.T) {
	// Arrange
	mockRepo := NewMockRepository[CacheTestEntity]()
	cache := NewGoCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	cachedRepo := NewRepositoryCache(mockRepo, cache, DefaultCacheConfig())
	ctx := context.Background()

	entityID := uuid.New()
	entities := []*CacheTestEntity{
		{ID: entityID, Name: "Entity 1"},
		{ID: uuid.New(), Name: "Entity 2"},
	}
	remainingEntities := entities[1:]

	callCount := 0
	mockRepo.getFunc = func(ctx context.Context) ([]*CacheTestEntity, error) {
		callCount++
		if callCount == 1 {
			return entities, nil
		}
		return remainingEntities, nil
	}
	mockRepo.deleteFunc = func(ctx context.Context, id uuid.UUID) error {
		return nil
	}

	// Populate cache
	_, _ = cachedRepo.Get(ctx)

	// Act
	err := cachedRepo.Delete(ctx, entityID)
	require.NoError(t, err)

	// Get should fetch from database again
	result, err := cachedRepo.Get(ctx)
	require.NoError(t, err)
	assert.Len(t, result, 1)

	// Assert
	assert.Equal(t, 2, mockRepo.GetCallCount("Get"))
}

// =============================================================================
// TTL Expiry Tests
// =============================================================================

func TestRepositoryCache_TTLExpiry_RefetchesFromDatabase(t *testing.T) {
	// Arrange
	mockRepo := NewMockRepository[CacheTestEntity]()
	config := CacheConfig{
		Enabled:         true,
		DefaultTTL:      50 * time.Millisecond,
		CleanupInterval: 10 * time.Millisecond,
	}
	cache := NewGoCache(config)
	require.NotNil(t, cache)
	cachedRepo := NewRepositoryCache(mockRepo, cache, config)
	ctx := context.Background()

	entityID := uuid.New()
	entity := &CacheTestEntity{ID: entityID, Name: "Entity"}

	mockRepo.getByIDFunc = func(ctx context.Context, id uuid.UUID) (*CacheTestEntity, error) {
		return entity, nil
	}

	// Populate cache
	_, _ = cachedRepo.GetByID(ctx, entityID)

	// Wait for TTL expiry
	time.Sleep(100 * time.Millisecond)

	// Act - Should fetch from database again
	_, err := cachedRepo.GetByID(ctx, entityID)
	require.NoError(t, err)

	// Assert - Called twice (once before TTL, once after)
	assert.Equal(t, 2, mockRepo.GetCallCount("GetByID"))
}

// =============================================================================
// Wrapper Transparency Tests
// =============================================================================

func TestRepositoryCache_Exists_PassesThrough(t *testing.T) {
	// Arrange
	mockRepo := NewMockRepository[CacheTestEntity]()
	cache := NewGoCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	cachedRepo := NewRepositoryCache(mockRepo, cache, DefaultCacheConfig())
	ctx := context.Background()

	entityID := uuid.New()
	mockRepo.existsFunc = func(ctx context.Context, id uuid.UUID) (bool, error) {
		return true, nil
	}

	// Act
	exists, err := cachedRepo.Exists(ctx, entityID)

	// Assert
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, 1, mockRepo.GetCallCount("Exists"))
}

func TestRepositoryCache_Count_PassesThrough(t *testing.T) {
	// Arrange
	mockRepo := NewMockRepository[CacheTestEntity]()
	cache := NewGoCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	cachedRepo := NewRepositoryCache(mockRepo, cache, DefaultCacheConfig())
	ctx := context.Background()

	mockRepo.countFunc = func(ctx context.Context) (int64, error) {
		return int64(42), nil
	}

	// Act
	count, err := cachedRepo.Count(ctx)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(42), count)
	assert.Equal(t, 1, mockRepo.GetCallCount("Count"))
}

func TestRepositoryCache_Close_ClosesUnderlyingRepository(t *testing.T) {
	// Arrange
	mockRepo := NewMockRepository[CacheTestEntity]()
	cache := NewGoCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	cachedRepo := NewRepositoryCache(mockRepo, cache, DefaultCacheConfig())

	mockRepo.closeFunc = func() error {
		return nil
	}

	// Act
	err := cachedRepo.Close()

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 1, mockRepo.GetCallCount("Close"))
}

// =============================================================================
// Concurrency Safety Tests
// =============================================================================

func TestRepositoryCache_ConcurrentGetByID(t *testing.T) {
	// Arrange
	mockRepo := NewMockRepository[CacheTestEntity]()
	cache := NewGoCache(DefaultCacheConfig())
	require.NotNil(t, cache)
	cachedRepo := NewRepositoryCache(mockRepo, cache, DefaultCacheConfig())
	ctx := context.Background()

	entityID := uuid.New()
	entity := &CacheTestEntity{ID: entityID, Name: "Concurrent Entity"}

	// Allow multiple calls but we expect caching to reduce them
	mockRepo.getByIDFunc = func(ctx context.Context, id uuid.UUID) (*CacheTestEntity, error) {
		return entity, nil
	}

	// Act - 50 concurrent requests for same entity
	var wg sync.WaitGroup
	const numGoroutines = 50
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			result, err := cachedRepo.GetByID(ctx, entityID)
			assert.NoError(t, err)
			assert.Equal(t, entity.Name, result.Name)
		}()
	}

	wg.Wait()

	// Assert - Database should be called very few times due to caching
	// (ideally once, but race conditions might cause a few more)
	calls := mockRepo.GetCallCount("GetByID")
	assert.Less(t, calls, numGoroutines,
		"Expected fewer database calls than concurrent requests due to caching")
}

// =============================================================================
// Cache Disabled Tests
// =============================================================================

func TestRepositoryCache_Disabled_ReturnsBaseRepository(t *testing.T) {
	// Arrange
	mockRepo := NewMockRepository[CacheTestEntity]()
	config := CacheConfig{
		Enabled: false,
	}
	cachedRepo := NewRepositoryCache(mockRepo, nil, config)

	// Assert - Should return the base repository directly
	assert.Equal(t, mockRepo, cachedRepo)
}

