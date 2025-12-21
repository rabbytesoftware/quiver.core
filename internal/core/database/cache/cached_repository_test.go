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

type MockRepository struct {
	mu       sync.RWMutex
	entities map[uuid.UUID]*TestEntity

	GetCalls     int
	GetByIDCalls int
	CreateCalls  int
	UpdateCalls  int
	DeleteCalls  int
	ExistsCalls  int
	CountCalls   int

	GetError     error
	GetByIDError error
	CreateError  error
	UpdateError  error
	DeleteError  error
	ExistsError  error
	CountError   error
}

func NewMockRepository() *MockRepository {
	return &MockRepository{
		entities: make(map[uuid.UUID]*TestEntity),
	}
}

func (m *MockRepository) Get(ctx context.Context) ([]*TestEntity, error) {
	m.mu.Lock()
	m.GetCalls++
	m.mu.Unlock()

	if m.GetError != nil {
		return nil, m.GetError
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*TestEntity, 0, len(m.entities))
	for _, entity := range m.entities {
		copied := *entity
		result = append(result, &copied)
	}
	return result, nil
}

func (m *MockRepository) GetByID(ctx context.Context, id uuid.UUID) (*TestEntity, error) {
	m.mu.Lock()
	m.GetByIDCalls++
	m.mu.Unlock()

	if m.GetByIDError != nil {
		return nil, m.GetByIDError
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	entity, exists := m.entities[id]
	if !exists {
		return nil, fmt.Errorf("entity with id %s not found", id)
	}
	copied := *entity
	return &copied, nil
}

func (m *MockRepository) Create(ctx context.Context, entity *TestEntity) (*TestEntity, error) {
	m.mu.Lock()
	m.CreateCalls++
	m.mu.Unlock()

	if m.CreateError != nil {
		return nil, m.CreateError
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	copied := *entity
	m.entities[entity.ID] = &copied
	return entity, nil
}

func (m *MockRepository) Update(ctx context.Context, entity *TestEntity) (*TestEntity, error) {
	m.mu.Lock()
	m.UpdateCalls++
	m.mu.Unlock()

	if m.UpdateError != nil {
		return nil, m.UpdateError
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.entities[entity.ID]; !exists {
		return nil, fmt.Errorf("entity with id %s not found", entity.ID)
	}
	copied := *entity
	m.entities[entity.ID] = &copied
	return entity, nil
}

func (m *MockRepository) Delete(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	m.DeleteCalls++
	m.mu.Unlock()

	if m.DeleteError != nil {
		return m.DeleteError
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.entities, id)
	return nil
}

func (m *MockRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	m.mu.Lock()
	m.ExistsCalls++
	m.mu.Unlock()

	if m.ExistsError != nil {
		return false, m.ExistsError
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.entities[id]
	return exists, nil
}

func (m *MockRepository) Count(ctx context.Context) (int64, error) {
	m.mu.Lock()
	m.CountCalls++
	m.mu.Unlock()

	if m.CountError != nil {
		return 0, m.CountError
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	return int64(len(m.entities)), nil
}

func (m *MockRepository) Close() error {
	return nil
}

func (m *MockRepository) ResetCounts() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GetCalls = 0
	m.GetByIDCalls = 0
	m.CreateCalls = 0
	m.UpdateCalls = 0
	m.DeleteCalls = 0
	m.ExistsCalls = 0
	m.CountCalls = 0
}

func TestNewCachedRepository_DisabledCache(t *testing.T) {
	mockRepo := NewMockRepository()
	config := CacheConfig{
		Enabled: false,
	}

	result, err := NewCachedRepository[TestEntity](mockRepo, config)

	require.NoError(t, err)
	assert.Equal(t, mockRepo, result, "Should return base repo when cache is disabled")
}

func TestNewCachedRepository_InvalidConfig(t *testing.T) {
	mockRepo := NewMockRepository()
	config := CacheConfig{
		Enabled:         true,
		DefaultTTL:      0,
		CleanupInterval: time.Minute,
	}

	result, err := NewCachedRepository[TestEntity](mockRepo, config)

	assert.ErrorIs(t, err, cacheerr.ErrInvalidCacheConfig)
	assert.Equal(t, mockRepo, result, "Should return base repo on invalid config")
}

func TestNewCachedRepository_ValidConfig(t *testing.T) {
	mockRepo := NewMockRepository()
	config := DefaultCacheConfig()

	result, err := NewCachedRepository[TestEntity](mockRepo, config)

	require.NoError(t, err)
	assert.NotEqual(t, mockRepo, result, "Should return CachedRepository wrapper")

	cachedRepo, ok := result.(*CachedRepository[TestEntity])
	require.True(t, ok, "Result should be *CachedRepository")
	assert.NotNil(t, cachedRepo.cache)
	assert.Equal(t, config, cachedRepo.config)
}

func TestCachedRepository_Create(t *testing.T) {
	mockRepo := NewMockRepository()
	cachedRepo := setupCachedRepo(t, mockRepo)
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
	assert.Equal(t, 1, mockRepo.CreateCalls, "Should delegate to base repo")
}

func TestCachedRepository_Create_BaseError(t *testing.T) {
	mockRepo := NewMockRepository()
	mockRepo.CreateError = fmt.Errorf("database error")
	cachedRepo := setupCachedRepo(t, mockRepo)
	ctx := context.Background()

	entity := &TestEntity{ID: uuid.New(), Name: "Test"}

	_, err := cachedRepo.Create(ctx, entity)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database error")
}

func TestCachedRepository_GetByID_CacheMiss(t *testing.T) {
	mockRepo := NewMockRepository()
	cachedRepo := setupCachedRepo(t, mockRepo)
	ctx := context.Background()

	entity := &TestEntity{ID: uuid.New(), Name: "Test Entity", Age: 30}
	mockRepo.entities[entity.ID] = entity

	result, err := cachedRepo.GetByID(ctx, entity.ID)

	require.NoError(t, err)
	assert.Equal(t, entity.ID, result.ID)
	assert.Equal(t, entity.Name, result.Name)
	assert.Equal(t, 1, mockRepo.GetByIDCalls, "Should call base repo on cache miss")
}

func TestCachedRepository_GetByID_CacheHit(t *testing.T) {
	mockRepo := NewMockRepository()
	cachedRepo := setupCachedRepo(t, mockRepo)
	ctx := context.Background()

	entity := &TestEntity{ID: uuid.New(), Name: "Test Entity", Age: 30}
	mockRepo.entities[entity.ID] = entity

	_, err := cachedRepo.GetByID(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, mockRepo.GetByIDCalls)

	result, err := cachedRepo.GetByID(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.ID, result.ID)
	assert.Equal(t, 1, mockRepo.GetByIDCalls, "Should NOT call base repo on cache hit")
}

func TestCachedRepository_GetByID_BaseError(t *testing.T) {
	mockRepo := NewMockRepository()
	mockRepo.GetByIDError = fmt.Errorf("not found")
	cachedRepo := setupCachedRepo(t, mockRepo)
	ctx := context.Background()

	_, err := cachedRepo.GetByID(ctx, uuid.New())

	assert.Error(t, err)
}

func TestCachedRepository_Get_CachesIndividually(t *testing.T) {
	mockRepo := NewMockRepository()
	cachedRepo := setupCachedRepo(t, mockRepo)
	ctx := context.Background()

	entities := []*TestEntity{
		{ID: uuid.New(), Name: "Entity 1", Age: 25},
		{ID: uuid.New(), Name: "Entity 2", Age: 30},
		{ID: uuid.New(), Name: "Entity 3", Age: 35},
	}
	for _, e := range entities {
		mockRepo.entities[e.ID] = e
	}

	result, err := cachedRepo.Get(ctx)
	require.NoError(t, err)
	assert.Len(t, result, 3)
	assert.Equal(t, 1, mockRepo.GetCalls)

	for _, entity := range entities {
		_, err := cachedRepo.GetByID(ctx, entity.ID)
		require.NoError(t, err)
	}
	assert.Equal(t, 0, mockRepo.GetByIDCalls, "GetByID should hit cache after Get")
}

func TestCachedRepository_Get_BaseError(t *testing.T) {
	mockRepo := NewMockRepository()
	mockRepo.GetError = fmt.Errorf("database error")
	cachedRepo := setupCachedRepo(t, mockRepo)
	ctx := context.Background()

	_, err := cachedRepo.Get(ctx)

	assert.Error(t, err)
}

func TestCachedRepository_Update_InvalidatesCache(t *testing.T) {
	mockRepo := NewMockRepository()
	cachedRepo := setupCachedRepo(t, mockRepo)
	ctx := context.Background()

	entity := &TestEntity{ID: uuid.New(), Name: "Original", Age: 25}
	mockRepo.entities[entity.ID] = entity

	_, err := cachedRepo.GetByID(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, mockRepo.GetByIDCalls)

	entity.Name = "Updated"
	_, err = cachedRepo.Update(ctx, entity)
	require.NoError(t, err)

	mockRepo.ResetCounts()
	_, err = cachedRepo.GetByID(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, mockRepo.GetByIDCalls, "Should call base after cache invalidation")
}

func TestCachedRepository_Update_BaseError(t *testing.T) {
	mockRepo := NewMockRepository()
	mockRepo.UpdateError = fmt.Errorf("update failed")
	cachedRepo := setupCachedRepo(t, mockRepo)
	ctx := context.Background()

	entity := &TestEntity{ID: uuid.New(), Name: "Test"}

	_, err := cachedRepo.Update(ctx, entity)

	assert.Error(t, err)
}

func TestCachedRepository_Delete_InvalidatesCache(t *testing.T) {
	mockRepo := NewMockRepository()
	cachedRepo := setupCachedRepo(t, mockRepo)
	ctx := context.Background()

	entity := &TestEntity{ID: uuid.New(), Name: "ToDelete", Age: 25}
	mockRepo.entities[entity.ID] = entity

	_, err := cachedRepo.GetByID(ctx, entity.ID)
	require.NoError(t, err)

	err = cachedRepo.Delete(ctx, entity.ID)
	require.NoError(t, err)

	_, err = cachedRepo.GetByID(ctx, entity.ID)
	assert.Error(t, err, "Should not find deleted entity")
}

func TestCachedRepository_Delete_BaseError(t *testing.T) {
	mockRepo := NewMockRepository()
	mockRepo.DeleteError = fmt.Errorf("delete failed")
	cachedRepo := setupCachedRepo(t, mockRepo)
	ctx := context.Background()

	err := cachedRepo.Delete(ctx, uuid.New())

	assert.Error(t, err)
}

func TestCachedRepository_Exists(t *testing.T) {
	mockRepo := NewMockRepository()
	cachedRepo := setupCachedRepo(t, mockRepo)
	ctx := context.Background()

	entity := &TestEntity{ID: uuid.New(), Name: "Test"}
	mockRepo.entities[entity.ID] = entity

	exists, err := cachedRepo.Exists(ctx, entity.ID)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, 1, mockRepo.ExistsCalls, "Should delegate to base")

	exists, err = cachedRepo.Exists(ctx, uuid.New())
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestCachedRepository_Count(t *testing.T) {
	mockRepo := NewMockRepository()
	cachedRepo := setupCachedRepo(t, mockRepo)
	ctx := context.Background()

	mockRepo.entities[uuid.New()] = &TestEntity{Name: "One"}
	mockRepo.entities[uuid.New()] = &TestEntity{Name: "Two"}

	count, err := cachedRepo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
	assert.Equal(t, 1, mockRepo.CountCalls, "Should delegate to base")
}

func TestCachedRepository_NilBase_ReturnsError(t *testing.T) {
	cachedRepo := &CachedRepository[TestEntity]{
		base:  nil,
		cache: nil,
	}
	ctx := context.Background()

	_, err := cachedRepo.Create(ctx, &TestEntity{})
	assert.ErrorIs(t, err, cacheerr.ErrMissingBase)

	_, err = cachedRepo.Get(ctx)
	assert.ErrorIs(t, err, cacheerr.ErrMissingCache)

	_, err = cachedRepo.Update(ctx, &TestEntity{})
	assert.ErrorIs(t, err, cacheerr.ErrMissingBase)

	err = cachedRepo.Delete(ctx, uuid.New())
	assert.ErrorIs(t, err, cacheerr.ErrMissingBase)
}

func TestCachedRepository_NilCache_ReturnsError(t *testing.T) {
	mockRepo := NewMockRepository()
	cachedRepo := &CachedRepository[TestEntity]{
		base:  mockRepo,
		cache: nil,
	}
	ctx := context.Background()

	_, err := cachedRepo.Get(ctx)
	assert.ErrorIs(t, err, cacheerr.ErrMissingCache)

	_, err = cachedRepo.GetByID(ctx, uuid.New())
	assert.ErrorIs(t, err, cacheerr.ErrMissingCache)
}

func TestCachedRepository_Integration_CRUD(t *testing.T) {
	baseRepo, cachedRepo := setupIntegrationRepos(t)
	_ = baseRepo
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

	baseRepo, err := repository.NewRepository[TestEntity](
		fmt.Sprintf("cache_expiry_test_%d", time.Now().UnixNano()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = baseRepo.Close() })

	config := CacheConfig{
		Enabled:         true,
		DefaultTTL:      100 * time.Millisecond,
		CleanupInterval: 50 * time.Millisecond,
	}
	cachedRepo, err := NewCachedRepository[TestEntity](baseRepo, config)
	require.NoError(t, err)

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
	_, cachedRepo := setupIntegrationRepos(t)
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

	id := uuid.New()
	entity := &TestEntity{ID: id, Name: "Parity Test", Age: 30}

	_, err := baseRepo.Create(ctx, entity)
	require.NoError(t, err)
	_, err = cachedRepo.Create(ctx, entity)
	require.NoError(t, err)

	baseResult, baseErr := baseRepo.GetByID(ctx, id)
	cachedResult, cachedErr := cachedRepo.GetByID(ctx, id)

	require.NoError(t, baseErr)
	require.NoError(t, cachedErr)
	assert.Equal(t, baseResult.ID, cachedResult.ID)
	assert.Equal(t, baseResult.Name, cachedResult.Name)
	assert.Equal(t, baseResult.Age, cachedResult.Age)
}

func TestParity_Get(t *testing.T) {
	baseRepo, cachedRepo := setupParityRepos(t)
	ctx := context.Background()

	entities := []*TestEntity{
		{ID: uuid.New(), Name: "Entity 1", Age: 25},
		{ID: uuid.New(), Name: "Entity 2", Age: 30},
	}

	for _, e := range entities {
		_, err := baseRepo.Create(ctx, e)
		require.NoError(t, err)
		_, err = cachedRepo.Create(ctx, e)
		require.NoError(t, err)
	}

	baseResult, baseErr := baseRepo.Get(ctx)
	cachedResult, cachedErr := cachedRepo.Get(ctx)

	require.NoError(t, baseErr)
	require.NoError(t, cachedErr)
	assert.Len(t, cachedResult, len(baseResult))

	baseNames := make(map[string]bool)
	for _, e := range baseResult {
		baseNames[e.Name] = true
	}
	for _, e := range cachedResult {
		assert.True(t, baseNames[e.Name], "Cached result should contain %s", e.Name)
	}
}

func TestParity_Update(t *testing.T) {
	baseRepo, cachedRepo := setupParityRepos(t)
	ctx := context.Background()

	id := uuid.New()
	entity := &TestEntity{ID: id, Name: "Original", Age: 25}

	_, err := baseRepo.Create(ctx, entity)
	require.NoError(t, err)
	_, err = cachedRepo.Create(ctx, entity)
	require.NoError(t, err)

	entity.Name = "Updated"
	baseResult, baseErr := baseRepo.Update(ctx, entity)
	cachedResult, cachedErr := cachedRepo.Update(ctx, entity)

	require.NoError(t, baseErr)
	require.NoError(t, cachedErr)
	assert.Equal(t, baseResult.Name, cachedResult.Name)
}

func TestParity_Exists(t *testing.T) {
	baseRepo, cachedRepo := setupParityRepos(t)
	ctx := context.Background()

	id := uuid.New()
	entity := &TestEntity{ID: id, Name: "Test", Age: 25}

	baseExists, _ := baseRepo.Exists(ctx, id)
	cachedExists, _ := cachedRepo.Exists(ctx, id)
	assert.Equal(t, baseExists, cachedExists)

	_, _ = baseRepo.Create(ctx, entity)
	_, _ = cachedRepo.Create(ctx, entity)

	baseExists, _ = baseRepo.Exists(ctx, id)
	cachedExists, _ = cachedRepo.Exists(ctx, id)
	assert.Equal(t, baseExists, cachedExists)
}

func TestParity_Count(t *testing.T) {
	baseRepo, cachedRepo := setupParityRepos(t)
	ctx := context.Background()

	baseCount, _ := baseRepo.Count(ctx)
	cachedCount, _ := cachedRepo.Count(ctx)
	assert.Equal(t, baseCount, cachedCount)

	for i := 0; i < 3; i++ {
		entity := &TestEntity{ID: uuid.New(), Name: fmt.Sprintf("Entity %d", i), Age: 20 + i}
		_, _ = baseRepo.Create(ctx, entity)
		_, _ = cachedRepo.Create(ctx, entity)
	}

	baseCount, _ = baseRepo.Count(ctx)
	cachedCount, _ = cachedRepo.Count(ctx)
	assert.Equal(t, baseCount, cachedCount)
}

func setupCachedRepo(t *testing.T, mockRepo *MockRepository) interfaces.RepositoryInterface[TestEntity] {
	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[TestEntity](mockRepo, config)
	require.NoError(t, err)
	return cachedRepo
}

func setupIntegrationRepos(t *testing.T) (
	interfaces.RepositoryInterface[TestEntity],
	interfaces.RepositoryInterface[TestEntity],
) {
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	dbName := fmt.Sprintf("integration_test_%d", time.Now().UnixNano())
	baseRepo, err := repository.NewRepository[TestEntity](dbName)
	require.NoError(t, err)
	t.Cleanup(func() { _ = baseRepo.Close() })

	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[TestEntity](baseRepo, config)
	require.NoError(t, err)

	return baseRepo, cachedRepo
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
	cachedBaseRepo, err := repository.NewRepository[TestEntity](cachedDbName)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cachedBaseRepo.Close() })

	config := DefaultCacheConfig()
	cachedRepo, err := NewCachedRepository[TestEntity](cachedBaseRepo, config)
	require.NoError(t, err)

	return baseRepo, cachedRepo
}
