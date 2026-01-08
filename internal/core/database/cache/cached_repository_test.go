package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cacheerr "github.com/rabbytesoftware/quiver/internal/core/database/cache/error"
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

// ExtractKey functions for different entity types
func testEntityExtractKey(entity *TestEntity) (string, error) {
	if entity == nil {
		return "", errors.New("nil entity")
	}
	return entity.ID.String(), nil
}

func entityWithoutIDExtractKey(entity *EntityWithoutID) (string, error) {
	return "", errors.New("entity has no ID field")
}

func entityWithWrongIDTypeExtractKey(entity *EntityWithWrongIDType) (string, error) {
	if entity == nil {
		return "", errors.New("nil entity")
	}
	return entity.ID, nil
}

func TestNewCachedRepository_ValidConfig(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	config := DefaultCacheConfig()
	dbName := fmt.Sprintf("valid_config_test_%d", time.Now().UnixNano())

	baseRepo, err := repository.NewRepository[TestEntity](dbName)
	require.NoError(t, err)

	result, err := NewCachedRepository[TestEntity](baseRepo, config, testEntityExtractKey)

	require.NoError(t, err)
	assert.NotNil(t, result, "Should return CachedRepository")

	cachedRepo, ok := result.(*CachedRepository[TestEntity])
	require.True(t, ok, "Result should be *CachedRepository")
	assert.NotNil(t, cachedRepo.cache)
	assert.NotNil(t, cachedRepo.db)
	assert.Equal(t, config, cachedRepo.config)

	t.Cleanup(func() { _ = result.Close() })
}

func TestNewCachedRepository_NilBaseRepo(t *testing.T) {
	config := DefaultCacheConfig()

	result, err := NewCachedRepository[TestEntity](nil, config, testEntityExtractKey)

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.ErrorIs(t, err, cacheerr.ErrMissingBase)
}

func TestNewCachedRepository_InvalidTTL(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	dbName := fmt.Sprintf("invalid_ttl_test_%d", time.Now().UnixNano())
	baseRepo, err := repository.NewRepository[TestEntity](dbName)
	require.NoError(t, err)
	t.Cleanup(func() { _ = baseRepo.Close() })

	config := CacheConfig{
		DefaultTTL:      0, // Invalid: must be > 0
		CleanupInterval: 10 * time.Minute,
	}

	result, err := NewCachedRepository[TestEntity](baseRepo, config, testEntityExtractKey)

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.ErrorIs(t, err, cacheerr.ErrInvalidCacheConfig)
}

func TestNewCachedRepository_InvalidCleanupInterval(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	dbName := fmt.Sprintf("invalid_cleanup_test_%d", time.Now().UnixNano())
	baseRepo, err := repository.NewRepository[TestEntity](dbName)
	require.NoError(t, err)
	t.Cleanup(func() { _ = baseRepo.Close() })

	config := CacheConfig{
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: -1 * time.Second, // Invalid: must be > 0
	}

	result, err := NewCachedRepository[TestEntity](baseRepo, config, testEntityExtractKey)

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.ErrorIs(t, err, cacheerr.ErrInvalidCacheConfig)
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

	_, err = cachedRepo.GetByID(ctx, entity.ID)
	require.NoError(t, err)

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

	_, err = cachedRepo.GetByID(ctx, entity.ID)
	require.NoError(t, err)

	entity.Name = "Updated"
	_, err = cachedRepo.Update(ctx, entity)
	require.NoError(t, err)

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

	_, err = cachedRepo.GetByID(ctx, entity.ID)
	require.NoError(t, err)

	err = cachedRepo.Delete(ctx, entity.ID)
	require.NoError(t, err)

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

	entity := &TestEntity{
		ID:   uuid.New(),
		Name: "Integration Test",
		Age:  30,
	}
	created, err := cachedRepo.Create(ctx, entity)
	require.NoError(t, err)
	assert.Equal(t, entity.ID, created.ID)

	retrieved, err := cachedRepo.GetByID(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.Name, retrieved.Name)

	entity.Name = "Updated Name"
	updated, err := cachedRepo.Update(ctx, entity)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", updated.Name)

	retrieved, err = cachedRepo.GetByID(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", retrieved.Name)

	err = cachedRepo.Delete(ctx, entity.ID)
	require.NoError(t, err)

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

	cachedRepo, err := NewCachedRepository[TestEntity](baseRepo, config, testEntityExtractKey)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cachedRepo.Close() })

	ctx := context.Background()

	entity := &TestEntity{ID: uuid.New(), Name: "Expiry Test", Age: 25}
	_, err = cachedRepo.Create(ctx, entity)
	require.NoError(t, err)

	_, err = cachedRepo.GetByID(ctx, entity.ID)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

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

// MockRepository for testing error paths
type MockRepository[T any] struct {
	GetFunc     func(ctx context.Context) ([]*T, error)
	GetByIDFunc func(ctx context.Context, id uuid.UUID) (*T, error)
	CreateFunc  func(ctx context.Context, entity *T) (*T, error)
	UpdateFunc  func(ctx context.Context, entity *T) (*T, error)
	DeleteFunc  func(ctx context.Context, id uuid.UUID) error
	ExistsFunc  func(ctx context.Context, id uuid.UUID) (bool, error)
	CountFunc   func(ctx context.Context) (int64, error)
	WhereFunc   func(ctx context.Context, query string, args ...interface{}) ([]*T, error)
	CloseFunc   func() error
}

func (m *MockRepository[T]) Get(ctx context.Context) ([]*T, error) {
	if m.GetFunc != nil {
		return m.GetFunc(ctx)
	}
	return nil, nil
}

func (m *MockRepository[T]) GetByID(ctx context.Context, id uuid.UUID) (*T, error) {
	if m.GetByIDFunc != nil {
		return m.GetByIDFunc(ctx, id)
	}
	return nil, nil
}

func (m *MockRepository[T]) Create(ctx context.Context, entity *T) (*T, error) {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, entity)
	}
	return entity, nil
}

func (m *MockRepository[T]) Update(ctx context.Context, entity *T) (*T, error) {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, entity)
	}
	return entity, nil
}

func (m *MockRepository[T]) Delete(ctx context.Context, id uuid.UUID) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, id)
	}
	return nil
}

func (m *MockRepository[T]) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	if m.ExistsFunc != nil {
		return m.ExistsFunc(ctx, id)
	}
	return false, nil
}

func (m *MockRepository[T]) Count(ctx context.Context) (int64, error) {
	if m.CountFunc != nil {
		return m.CountFunc(ctx)
	}
	return 0, nil
}

func (m *MockRepository[T]) Where(ctx context.Context, query string, args ...interface{}) ([]*T, error) {
	if m.WhereFunc != nil {
		return m.WhereFunc(ctx, query, args...)
	}
	return nil, nil
}

func (m *MockRepository[T]) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

func TestCachedRepository_Get_CacheHit(t *testing.T) {
	// Test that Get returns cached data when available
	mockRepo := &MockRepository[TestEntity]{
		GetFunc: func(ctx context.Context) ([]*TestEntity, error) {
			// This should NOT be called if cache hit
			return nil, errors.New("should not reach database")
		},
	}

	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[TestEntity](mockRepo, config, testEntityExtractKey)
	require.NoError(t, err)

	cr := cachedRepo.(*CachedRepository[TestEntity])

	// Pre-populate cache with valid list data
	cachedEntities := []*TestEntity{
		{ID: uuid.New(), Name: "Cached1", Age: 25},
		{ID: uuid.New(), Name: "Cached2", Age: 30},
	}
	listJSON, err := json.Marshal(cachedEntities)
	require.NoError(t, err)
	cr.cache.Set("list:cache.TestEntity", listJSON, config.DefaultTTL)

	ctx := context.Background()
	result, err := cachedRepo.Get(ctx)

	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "Cached1", result[0].Name)
	assert.Equal(t, "Cached2", result[1].Name)
}

func TestCachedRepository_Get_InvalidCacheValue(t *testing.T) {
	mockRepo := &MockRepository[TestEntity]{
		GetFunc: func(ctx context.Context) ([]*TestEntity, error) {
			return []*TestEntity{}, nil
		},
	}

	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[TestEntity](mockRepo, config, testEntityExtractKey)
	require.NoError(t, err)

	cr := cachedRepo.(*CachedRepository[TestEntity])
	// Inject invalid value type into cache (not []byte)
	cr.cache.Set("list:cache.TestEntity", "invalid-string-not-bytes", config.DefaultTTL)

	ctx := context.Background()
	result, err := cachedRepo.Get(ctx)

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.ErrorIs(t, err, cacheerr.ErrInvalidCacheValue)
}

func TestCachedRepository_Get_UnmarshalError(t *testing.T) {
	mockRepo := &MockRepository[TestEntity]{
		GetFunc: func(ctx context.Context) ([]*TestEntity, error) {
			return []*TestEntity{}, nil
		},
	}

	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[TestEntity](mockRepo, config, testEntityExtractKey)
	require.NoError(t, err)

	cr := cachedRepo.(*CachedRepository[TestEntity])
	// Inject invalid JSON bytes into cache
	cr.cache.Set("list:cache.TestEntity", []byte("not-valid-json{{{"), config.DefaultTTL)

	ctx := context.Background()
	result, err := cachedRepo.Get(ctx)

	assert.Nil(t, result)
	assert.Error(t, err)
}

func TestCachedRepository_Get_DatabaseError(t *testing.T) {
	dbError := errors.New("database connection failed")
	mockRepo := &MockRepository[TestEntity]{
		GetFunc: func(ctx context.Context) ([]*TestEntity, error) {
			return nil, dbError
		},
	}

	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[TestEntity](mockRepo, config, testEntityExtractKey)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := cachedRepo.Get(ctx)

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.ErrorIs(t, err, dbError)
}

func TestCachedRepository_GetByID_InvalidCacheValue(t *testing.T) {
	mockRepo := &MockRepository[TestEntity]{}

	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[TestEntity](mockRepo, config, testEntityExtractKey)
	require.NoError(t, err)

	cr := cachedRepo.(*CachedRepository[TestEntity])
	testID := uuid.New()
	// Inject invalid value type into cache (not []byte)
	key := fmt.Sprintf("entity:cache.TestEntity:%s", testID.String())
	cr.cache.Set(key, 12345, config.DefaultTTL) // integer instead of []byte

	ctx := context.Background()
	result, err := cachedRepo.GetByID(ctx, testID)

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.ErrorIs(t, err, cacheerr.ErrInvalidCacheValue)
}

func TestCachedRepository_GetByID_UnmarshalError(t *testing.T) {
	mockRepo := &MockRepository[TestEntity]{}

	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[TestEntity](mockRepo, config, testEntityExtractKey)
	require.NoError(t, err)

	cr := cachedRepo.(*CachedRepository[TestEntity])
	testID := uuid.New()
	// Inject invalid JSON bytes into cache
	key := fmt.Sprintf("entity:cache.TestEntity:%s", testID.String())
	cr.cache.Set(key, []byte("{invalid json"), config.DefaultTTL)

	ctx := context.Background()
	result, err := cachedRepo.GetByID(ctx, testID)

	assert.Nil(t, result)
	assert.Error(t, err)
}

func TestCachedRepository_Update_DatabaseError(t *testing.T) {
	dbError := errors.New("database update failed")
	mockRepo := &MockRepository[TestEntity]{
		UpdateFunc: func(ctx context.Context, entity *TestEntity) (*TestEntity, error) {
			return nil, dbError
		},
	}

	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[TestEntity](mockRepo, config, testEntityExtractKey)
	require.NoError(t, err)

	ctx := context.Background()
	entity := &TestEntity{ID: uuid.New(), Name: "Test", Age: 25}

	result, err := cachedRepo.Update(ctx, entity)

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.ErrorIs(t, err, dbError)
}

// EntityWithoutID is used to test ID extraction failure
type EntityWithoutID struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestCachedRepository_Update_IDExtractionFailed(t *testing.T) {
	mockRepo := &MockRepository[EntityWithoutID]{
		UpdateFunc: func(ctx context.Context, entity *EntityWithoutID) (*EntityWithoutID, error) {
			return entity, nil
		},
	}

	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[EntityWithoutID](mockRepo, config, entityWithoutIDExtractKey)
	require.NoError(t, err)

	ctx := context.Background()
	entity := &EntityWithoutID{Name: "Test", Age: 25}

	result, err := cachedRepo.Update(ctx, entity)

	// Update succeeds but returns error due to ID extraction failure
	assert.NotNil(t, result)
	assert.Error(t, err)
	assert.ErrorIs(t, err, cacheerr.ErrIDExtractionFailed)
}

func TestCachedRepository_Delete_DatabaseError(t *testing.T) {
	dbError := errors.New("database delete failed")
	mockRepo := &MockRepository[TestEntity]{
		DeleteFunc: func(ctx context.Context, id uuid.UUID) error {
			return dbError
		},
	}

	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[TestEntity](mockRepo, config, testEntityExtractKey)
	require.NoError(t, err)

	ctx := context.Background()
	err = cachedRepo.Delete(ctx, uuid.New())

	assert.Error(t, err)
	assert.ErrorIs(t, err, dbError)
}

func TestCachedRepository_Count_CacheHit(t *testing.T) {
	mockRepo := &MockRepository[TestEntity]{
		CountFunc: func(ctx context.Context) (int64, error) {
			return 999, nil // This should NOT be called if cache hit
		},
	}

	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[TestEntity](mockRepo, config, testEntityExtractKey)
	require.NoError(t, err)

	cr := cachedRepo.(*CachedRepository[TestEntity])
	// Pre-populate cache with count
	cr.cache.Set("list:cache.TestEntity", int64(42), config.DefaultTTL)

	ctx := context.Background()
	count, err := cachedRepo.Count(ctx)

	require.NoError(t, err)
	assert.Equal(t, int64(42), count)
}

func TestCachedRepository_Where_Success(t *testing.T) {
	expectedResults := []*TestEntity{
		{ID: uuid.New(), Name: "Match 1", Age: 25},
		{ID: uuid.New(), Name: "Match 2", Age: 30},
	}

	mockRepo := &MockRepository[TestEntity]{
		WhereFunc: func(ctx context.Context, query string, args ...interface{}) ([]*TestEntity, error) {
			return expectedResults, nil
		},
	}

	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[TestEntity](mockRepo, config, testEntityExtractKey)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := cachedRepo.Where(ctx, "age > ?", 20)

	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, expectedResults[0].Name, result[0].Name)
	assert.Equal(t, expectedResults[1].Name, result[1].Name)
}

func TestCachedRepository_Where_Error(t *testing.T) {
	dbError := errors.New("where query failed")
	mockRepo := &MockRepository[TestEntity]{
		WhereFunc: func(ctx context.Context, query string, args ...interface{}) ([]*TestEntity, error) {
			return nil, dbError
		},
	}

	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[TestEntity](mockRepo, config, testEntityExtractKey)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := cachedRepo.Where(ctx, "invalid query")

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.ErrorIs(t, err, dbError)
}

func TestParity_Where(t *testing.T) {
	baseRepo, cachedRepo := setupParityRepos(t)
	ctx := context.Background()

	// Create some entities
	for i := 0; i < 5; i++ {
		baseEntity := &TestEntity{ID: uuid.New(), Name: fmt.Sprintf("Entity %d", i), Age: 20 + i}
		cachedEntity := &TestEntity{ID: uuid.New(), Name: fmt.Sprintf("Entity %d", i), Age: 20 + i}
		_, err := baseRepo.Create(ctx, baseEntity)
		require.NoError(t, err)
		_, err = cachedRepo.Create(ctx, cachedEntity)
		require.NoError(t, err)
	}

	// Query with Where
	baseResult, baseErr := baseRepo.Where(ctx, "age >= ?", 22)
	cachedResult, cachedErr := cachedRepo.Where(ctx, "age >= ?", 22)

	// Both should succeed or fail together
	assert.Equal(t, baseErr == nil, cachedErr == nil, "Error status should match")
	if baseErr == nil && cachedErr == nil {
		assert.Equal(t, len(baseResult), len(cachedResult), "Result count should match")
	}
}

// EntityWithWrongIDType has an ID field but it's not uuid.UUID
type EntityWithWrongIDType struct {
	ID   string `json:"id"` // Wrong type: string instead of uuid.UUID
	Name string `json:"name"`
}

func TestExtractID_NilEntity(t *testing.T) {
	// When Get returns entities including nil, extractID should handle it gracefully
	mockRepo := &MockRepository[TestEntity]{
		GetFunc: func(ctx context.Context) ([]*TestEntity, error) {
			// Return a slice with a nil entity
			return []*TestEntity{nil, {ID: uuid.New(), Name: "Valid"}}, nil
		},
	}

	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[TestEntity](mockRepo, config, testEntityExtractKey)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := cachedRepo.Get(ctx)

	// Should not panic, but nil entity won't be cached
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

func TestExtractID_MissingIDField(t *testing.T) {
	// Test with entity that has no ID field - ID extraction should fail gracefully
	mockRepo := &MockRepository[EntityWithoutID]{
		GetFunc: func(ctx context.Context) ([]*EntityWithoutID, error) {
			return []*EntityWithoutID{
				{Name: "Entity1", Age: 25},
				{Name: "Entity2", Age: 30},
			}, nil
		},
	}

	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[EntityWithoutID](mockRepo, config, entityWithoutIDExtractKey)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := cachedRepo.Get(ctx)

	// Should succeed but entities won't be individually cached
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

func TestExtractID_WrongIDType(t *testing.T) {
	// Test with entity that has ID field but wrong type
	mockRepo := &MockRepository[EntityWithWrongIDType]{
		GetFunc: func(ctx context.Context) ([]*EntityWithWrongIDType, error) {
			return []*EntityWithWrongIDType{
				{ID: "string-id-1", Name: "Entity1"},
				{ID: "string-id-2", Name: "Entity2"},
			}, nil
		},
	}

	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[EntityWithWrongIDType](mockRepo, config, entityWithWrongIDTypeExtractKey)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := cachedRepo.Get(ctx)

	// Should succeed but entities won't be individually cached (ID extraction fails)
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

func TestExtractID_GetByID_EntityWithoutID(t *testing.T) {
	// Test GetByID with entity that doesn't have proper ID
	testEntity := &EntityWithoutID{Name: "NoID", Age: 25}
	mockRepo := &MockRepository[EntityWithoutID]{
		GetByIDFunc: func(ctx context.Context, id uuid.UUID) (*EntityWithoutID, error) {
			return testEntity, nil
		},
	}

	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[EntityWithoutID](mockRepo, config, entityWithoutIDExtractKey)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := cachedRepo.GetByID(ctx, uuid.New())

	// Should succeed but entity won't be cached
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "NoID", result.Name)
}

func TestCachedRepository_Exists_CacheHit(t *testing.T) {
	// Test that Exists returns true when entity is in cache
	mockRepo := &MockRepository[TestEntity]{
		ExistsFunc: func(ctx context.Context, id uuid.UUID) (bool, error) {
			// This should NOT be called if cache hit
			return false, errors.New("should not reach database")
		},
	}

	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[TestEntity](mockRepo, config, testEntityExtractKey)
	require.NoError(t, err)

	cr := cachedRepo.(*CachedRepository[TestEntity])
	testID := uuid.New()
	testEntity := &TestEntity{ID: testID, Name: "Cached", Age: 30}

	// Pre-populate cache with the entity (simulating a prior GetByID call)
	entityJSON, err := json.Marshal(testEntity)
	require.NoError(t, err)
	key := fmt.Sprintf("entity:cache.TestEntity:%s", testID.String())
	cr.cache.Set(key, entityJSON, config.DefaultTTL)

	ctx := context.Background()
	exists, err := cachedRepo.Exists(ctx, testID)

	require.NoError(t, err)
	assert.True(t, exists, "Exists should return true when entity is in cache")
}

func TestCachedRepository_Exists_CacheMiss_DBReturnsTrue(t *testing.T) {
	// Test that Exists delegates to DB when cache miss, DB returns true
	testID := uuid.New()
	mockRepo := &MockRepository[TestEntity]{
		ExistsFunc: func(ctx context.Context, id uuid.UUID) (bool, error) {
			if id == testID {
				return true, nil
			}
			return false, nil
		},
	}

	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[TestEntity](mockRepo, config, testEntityExtractKey)
	require.NoError(t, err)

	ctx := context.Background()
	exists, err := cachedRepo.Exists(ctx, testID)

	require.NoError(t, err)
	assert.True(t, exists)
}

func TestCachedRepository_Exists_CacheMiss_DBReturnsFalse(t *testing.T) {
	// Test that Exists delegates to DB when cache miss, DB returns false
	mockRepo := &MockRepository[TestEntity]{
		ExistsFunc: func(ctx context.Context, id uuid.UUID) (bool, error) {
			return false, nil
		},
	}

	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[TestEntity](mockRepo, config, testEntityExtractKey)
	require.NoError(t, err)

	ctx := context.Background()
	exists, err := cachedRepo.Exists(ctx, uuid.New())

	require.NoError(t, err)
	assert.False(t, exists)
}

func TestCachedRepository_Exists_DatabaseError(t *testing.T) {
	// Test that Exists propagates database errors
	dbError := errors.New("database error in exists")
	mockRepo := &MockRepository[TestEntity]{
		ExistsFunc: func(ctx context.Context, id uuid.UUID) (bool, error) {
			return false, dbError
		},
	}

	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[TestEntity](mockRepo, config, testEntityExtractKey)
	require.NoError(t, err)

	ctx := context.Background()
	exists, err := cachedRepo.Exists(ctx, uuid.New())

	assert.False(t, exists)
	assert.Error(t, err)
	assert.ErrorIs(t, err, dbError)
}

func setupCachedRepo(t *testing.T) interfaces.RepositoryInterface[TestEntity] {
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	config := DefaultCacheConfig()
	dbName := fmt.Sprintf("cached_repo_test_%d", time.Now().UnixNano())

	baseRepo, err := repository.NewRepository[TestEntity](dbName)
	require.NoError(t, err)

	cachedRepo, err := NewCachedRepository[TestEntity](baseRepo, config, testEntityExtractKey)
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

	cachedRepo, err := NewCachedRepository[TestEntity](cachedBaseRepo, config, testEntityExtractKey)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cachedRepo.Close() })

	return baseRepo, cachedRepo
}

func BenchmarkCachedRepository_Create(b *testing.B) {
	repo, cleanup := setupBenchmarkRepo(b)
	defer cleanup()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entity := &TestEntity{
			ID:   uuid.New(),
			Name: fmt.Sprintf("Benchmark Entity %d", i),
			Age:  25 + (i % 50),
		}
		_, _ = repo.Create(ctx, entity)
	}
}

func BenchmarkCachedRepository_GetByID_CacheMiss(b *testing.B) {
	repo, cleanup := setupBenchmarkRepo(b)
	defer cleanup()

	ctx := context.Background()

	ids := make([]uuid.UUID, b.N)
	for i := 0; i < b.N; i++ {
		entity := &TestEntity{
			ID:   uuid.New(),
			Name: fmt.Sprintf("Entity %d", i),
			Age:  25,
		}
		created, _ := repo.Create(ctx, entity)
		ids[i] = created.ID
	}

	cachedRepo := repo.(*CachedRepository[TestEntity])
	cachedRepo.cache.Flush()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = repo.GetByID(ctx, ids[i%len(ids)])
	}
}

func BenchmarkCachedRepository_GetByID_CacheHit(b *testing.B) {
	repo, cleanup := setupBenchmarkRepo(b)
	defer cleanup()

	ctx := context.Background()

	entity := &TestEntity{
		ID:   uuid.New(),
		Name: "Cached Entity",
		Age:  30,
	}
	created, _ := repo.Create(ctx, entity)

	_, _ = repo.GetByID(ctx, created.ID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = repo.GetByID(ctx, created.ID)
	}
}

func BenchmarkCachedRepository_Get(b *testing.B) {
	repo, cleanup := setupBenchmarkRepo(b)
	defer cleanup()

	ctx := context.Background()

	for i := 0; i < 100; i++ {
		entity := &TestEntity{
			ID:   uuid.New(),
			Name: fmt.Sprintf("Entity %d", i),
			Age:  20 + i,
		}
		_, _ = repo.Create(ctx, entity)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = repo.Get(ctx)
	}
}

func BenchmarkCachedRepository_Update(b *testing.B) {
	repo, cleanup := setupBenchmarkRepo(b)
	defer cleanup()

	ctx := context.Background()

	entity := &TestEntity{
		ID:   uuid.New(),
		Name: "Original",
		Age:  25,
	}
	created, _ := repo.Create(ctx, entity)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		created.Name = fmt.Sprintf("Updated %d", i)
		_, _ = repo.Update(ctx, created)
	}
}

func BenchmarkCachedRepository_Delete(b *testing.B) {
	repo, cleanup := setupBenchmarkRepo(b)
	defer cleanup()

	ctx := context.Background()

	ids := make([]uuid.UUID, b.N)
	for i := 0; i < b.N; i++ {
		entity := &TestEntity{
			ID:   uuid.New(),
			Name: fmt.Sprintf("ToDelete %d", i),
			Age:  25,
		}
		created, _ := repo.Create(ctx, entity)
		ids[i] = created.ID
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = repo.Delete(ctx, ids[i])
	}
}

func BenchmarkCachedRepository_Exists_CacheHit(b *testing.B) {
	repo, cleanup := setupBenchmarkRepo(b)
	defer cleanup()

	ctx := context.Background()

	entity := &TestEntity{
		ID:   uuid.New(),
		Name: "Exists Test",
		Age:  30,
	}
	created, _ := repo.Create(ctx, entity)

	_, _ = repo.GetByID(ctx, created.ID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = repo.Exists(ctx, created.ID)
	}
}

func BenchmarkCachedRepository_Count(b *testing.B) {
	repo, cleanup := setupBenchmarkRepo(b)
	defer cleanup()

	ctx := context.Background()

	for i := 0; i < 50; i++ {
		entity := &TestEntity{
			ID:   uuid.New(),
			Name: fmt.Sprintf("Entity %d", i),
			Age:  20 + i,
		}
		_, _ = repo.Create(ctx, entity)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = repo.Count(ctx)
	}
}

func BenchmarkComparison_GetByID_NonCached(b *testing.B) {
	repo, cleanup := setupBenchmarkBaseRepo(b)
	defer cleanup()

	ctx := context.Background()

	entity := &TestEntity{
		ID:   uuid.New(),
		Name: "Test Entity",
		Age:  30,
	}
	created, _ := repo.Create(ctx, entity)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = repo.GetByID(ctx, created.ID)
	}
}

func BenchmarkComparison_GetByID_Cached(b *testing.B) {
	repo, cleanup := setupBenchmarkRepo(b)
	defer cleanup()

	ctx := context.Background()

	entity := &TestEntity{
		ID:   uuid.New(),
		Name: "Test Entity",
		Age:  30,
	}
	created, _ := repo.Create(ctx, entity)

	_, _ = repo.GetByID(ctx, created.ID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = repo.GetByID(ctx, created.ID)
	}
}

func BenchmarkComparison_Exists_NonCached(b *testing.B) {
	repo, cleanup := setupBenchmarkBaseRepo(b)
	defer cleanup()

	ctx := context.Background()

	entity := &TestEntity{
		ID:   uuid.New(),
		Name: "Test Entity",
		Age:  30,
	}
	created, _ := repo.Create(ctx, entity)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = repo.Exists(ctx, created.ID)
	}
}

func BenchmarkComparison_Exists_Cached(b *testing.B) {
	repo, cleanup := setupBenchmarkRepo(b)
	defer cleanup()

	ctx := context.Background()

	entity := &TestEntity{
		ID:   uuid.New(),
		Name: "Test Entity",
		Age:  30,
	}
	created, _ := repo.Create(ctx, entity)

	_, _ = repo.GetByID(ctx, created.ID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = repo.Exists(ctx, created.ID)
	}
}

func setupBenchmarkRepo(b *testing.B) (interfaces.RepositoryInterface[TestEntity], func()) {
	b.Helper()

	tempDir := b.TempDir()
	b.Setenv("QUIVER_DATABASE_PATH", tempDir)

	config := DefaultCacheConfig()
	dbName := fmt.Sprintf("bench_cached_%d", time.Now().UnixNano())

	baseRepo, err := repository.NewRepository[TestEntity](dbName)
	if err != nil {
		b.Fatalf("Failed to create base repository: %v", err)
	}

	cachedRepo, err := NewCachedRepository[TestEntity](baseRepo, config, testEntityExtractKey)
	if err != nil {
		_ = baseRepo.Close()
		b.Fatalf("Failed to create cached repository: %v", err)
	}

	cleanup := func() {
		_ = cachedRepo.Close()
	}

	return cachedRepo, cleanup
}

func setupBenchmarkBaseRepo(b *testing.B) (interfaces.RepositoryInterface[TestEntity], func()) {
	b.Helper()

	tempDir := b.TempDir()
	b.Setenv("QUIVER_DATABASE_PATH", tempDir)

	dbName := fmt.Sprintf("bench_base_%d", time.Now().UnixNano())

	baseRepo, err := repository.NewRepository[TestEntity](dbName)
	if err != nil {
		b.Fatalf("Failed to create base repository: %v", err)
	}

	cleanup := func() {
		_ = baseRepo.Close()
	}

	return baseRepo, cleanup
}
