package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEntity represents a test entity for repository testing
type TestEntity struct {
	ID   uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	Name string    `gorm:"not null" json:"name"`
	Age  int       `json:"age"`
}

// TableName returns the table name for the TestEntity
func (TestEntity) TableName() string {
	return "test_entities"
}

func TestNewRepository(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Override the database path for testing
	originalPath := os.Getenv("QUIVER_DATABASE_PATH")
	defer func() {
		if originalPath != "" {
			os.Setenv("QUIVER_DATABASE_PATH", originalPath)
		} else {
			os.Unsetenv("QUIVER_DATABASE_PATH")
		}
	}()

	os.Setenv("QUIVER_DATABASE_PATH", tempDir)

	// Test repository creation
	repo, err := NewRepository[TestEntity]("test")
	require.NoError(t, err)
	assert.NotNil(t, repo)

	// Verify the repository has the correct name
	repoImpl := repo.(*Repository[TestEntity])
	assert.Equal(t, "test", repoImpl.name)
	assert.NotNil(t, repoImpl.db)
}

func TestRepository_Create(t *testing.T) {
	repo := setupTestRepository(t)
	ctx := context.Background()

	// Test creating a new entity
	entity := &TestEntity{
		ID:   uuid.New(),
		Name: "Test Entity",
		Age:  25,
	}

	created, err := repo.Create(ctx, entity)
	require.NoError(t, err)
	assert.NotNil(t, created)
	assert.NotEqual(t, uuid.Nil, created.ID)
	assert.Equal(t, "Test Entity", created.Name)
	assert.Equal(t, 25, created.Age)
}

func TestRepository_GetByID(t *testing.T) {
	repo := setupTestRepository(t)
	ctx := context.Background()

	// Create an entity first
	entity := &TestEntity{
		ID:   uuid.New(),
		Name: "Test Entity",
		Age:  25,
	}
	created, err := repo.Create(ctx, entity)
	require.NoError(t, err)

	// Test getting by ID
	retrieved, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, retrieved.ID)
	assert.Equal(t, created.Name, retrieved.Name)
	assert.Equal(t, created.Age, retrieved.Age)

	// Test getting non-existent entity
	nonExistentID := uuid.New()
	_, err = repo.GetByID(ctx, nonExistentID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRepository_Get(t *testing.T) {
	repo := setupTestRepository(t)
	ctx := context.Background()

	// Create multiple entities
	entities := []*TestEntity{
		{ID: uuid.New(), Name: "Entity 1", Age: 25},
		{ID: uuid.New(), Name: "Entity 2", Age: 30},
		{ID: uuid.New(), Name: "Entity 3", Age: 35},
	}

	for _, entity := range entities {
		_, err := repo.Create(ctx, entity)
		require.NoError(t, err)
	}

	// Test getting all entities
	allEntities, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.Len(t, allEntities, 3)

	// Verify all entities are present
	names := make(map[string]bool)
	for _, entity := range allEntities {
		names[entity.Name] = true
	}
	assert.True(t, names["Entity 1"])
	assert.True(t, names["Entity 2"])
	assert.True(t, names["Entity 3"])
}

func TestRepository_Update(t *testing.T) {
	repo := setupTestRepository(t)
	ctx := context.Background()

	// Create an entity
	entity := &TestEntity{
		ID:   uuid.New(),
		Name: "Original Name",
		Age:  25,
	}
	created, err := repo.Create(ctx, entity)
	require.NoError(t, err)

	// Update the entity
	created.Name = "Updated Name"
	created.Age = 30

	updated, err := repo.Update(ctx, created)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", updated.Name)
	assert.Equal(t, 30, updated.Age)
	assert.Equal(t, created.ID, updated.ID)

	// Verify the update persisted
	retrieved, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", retrieved.Name)
	assert.Equal(t, 30, retrieved.Age)
}

func TestRepository_Delete(t *testing.T) {
	repo := setupTestRepository(t)
	ctx := context.Background()

	// Create an entity
	entity := &TestEntity{
		ID:   uuid.New(),
		Name: "To Be Deleted",
		Age:  25,
	}
	created, err := repo.Create(ctx, entity)
	require.NoError(t, err)

	// Verify entity exists
	exists, err := repo.Exists(ctx, created.ID)
	require.NoError(t, err)
	assert.True(t, exists)

	// Delete the entity
	err = repo.Delete(ctx, created.ID)
	require.NoError(t, err)

	// Verify entity no longer exists
	exists, err = repo.Exists(ctx, created.ID)
	require.NoError(t, err)
	assert.False(t, exists)

	// Verify GetByID returns error
	_, err = repo.GetByID(ctx, created.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRepository_Exists(t *testing.T) {
	repo := setupTestRepository(t)
	ctx := context.Background()

	// Test non-existent entity
	nonExistentID := uuid.New()
	exists, err := repo.Exists(ctx, nonExistentID)
	require.NoError(t, err)
	assert.False(t, exists)

	// Create an entity
	entity := &TestEntity{
		ID:   uuid.New(),
		Name: "Test Entity",
		Age:  25,
	}
	created, err := repo.Create(ctx, entity)
	require.NoError(t, err)

	// Test existing entity
	exists, err = repo.Exists(ctx, created.ID)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestRepository_Count(t *testing.T) {
	repo := setupTestRepository(t)
	ctx := context.Background()

	// Test empty repository
	count, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Create multiple entities
	entities := []*TestEntity{
		{ID: uuid.New(), Name: "Entity 1", Age: 25},
		{ID: uuid.New(), Name: "Entity 2", Age: 30},
		{ID: uuid.New(), Name: "Entity 3", Age: 35},
	}

	for _, entity := range entities {
		_, err := repo.Create(ctx, entity)
		require.NoError(t, err)
	}

	// Test count after creating entities
	count, err = repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)

	// Delete one entity
	firstEntity, err := repo.Get(ctx)
	require.NoError(t, err)
	require.Len(t, firstEntity, 3)

	err = repo.Delete(ctx, firstEntity[0].ID)
	require.NoError(t, err)

	// Test count after deletion
	count, err = repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

func TestRepository_ContextCancellation(t *testing.T) {
	repo := setupTestRepository(t)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Test that operations respect context cancellation
	_, err := repo.Get(ctx)
	assert.Error(t, err)

	_, err = repo.GetByID(ctx, uuid.New())
	assert.Error(t, err)

	_, err = repo.Create(ctx, &TestEntity{Name: "Test"})
	assert.Error(t, err)

	_, err = repo.Update(ctx, &TestEntity{Name: "Test"})
	assert.Error(t, err)

	err = repo.Delete(ctx, uuid.New())
	assert.Error(t, err)

	_, err = repo.Exists(ctx, uuid.New())
	assert.Error(t, err)

	_, err = repo.Count(ctx)
	assert.Error(t, err)
}

// setupTestRepository creates a test repository with a temporary database
func setupTestRepository(t *testing.T) *Repository[TestEntity] {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Override the database path for testing
	originalPath := os.Getenv("QUIVER_DATABASE_PATH")
	defer func() {
		if originalPath != "" {
			os.Setenv("QUIVER_DATABASE_PATH", originalPath)
		} else {
			os.Unsetenv("QUIVER_DATABASE_PATH")
		}
	}()

	os.Setenv("QUIVER_DATABASE_PATH", tempDir)

	// Create repository with unique name for each test
	uniqueName := fmt.Sprintf("test_%s_%d", t.Name(), time.Now().UnixNano())
	repo, err := NewRepository[TestEntity](uniqueName)
	require.NoError(t, err)
	return repo.(*Repository[TestEntity])
}

// TestRepository_ConcurrentOperations tests concurrent access to the repository
func TestRepository_ConcurrentOperations(t *testing.T) {
	repo := setupTestRepository(t)
	ctx := context.Background()

	// Create multiple entities concurrently
	const numEntities = 10
	done := make(chan error, numEntities)

	for i := 0; i < numEntities; i++ {
		go func(i int) {
			entity := &TestEntity{
				ID:   uuid.New(),
				Name: fmt.Sprintf("Concurrent Entity %d", i),
				Age:  20 + i,
			}
			_, err := repo.Create(ctx, entity)
			done <- err
		}(i)
	}

	// Wait for all operations to complete
	for i := 0; i < numEntities; i++ {
		err := <-done
		require.NoError(t, err)
	}

	// Verify all entities were created
	count, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(numEntities), count)

	// Verify we can retrieve all entities
	entities, err := repo.Get(ctx)
	require.NoError(t, err)
	assert.Len(t, entities, numEntities)
}
