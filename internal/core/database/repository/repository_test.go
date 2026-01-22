package repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestEntity struct {
	ID   uuid.UUID `gorm:"type:uuid;primary_key" json:"id"`
	Name string    `gorm:"not null" json:"name"`
	Age  int       `json:"age"`
}

func (TestEntity) TableName() string {
	return "test_entities"
}

func TestNewRepository(t *testing.T) {
	tempDir := t.TempDir()

	originalPath := os.Getenv("QUIVER_DATABASE_PATH")
	defer func() {
		if originalPath != "" {
			os.Setenv("QUIVER_DATABASE_PATH", originalPath)
		} else {
			os.Unsetenv("QUIVER_DATABASE_PATH")
		}
	}()

	os.Setenv("QUIVER_DATABASE_PATH", tempDir)

	repo, err := NewRepository[TestEntity]("test")
	require.NoError(t, err)
	assert.NotNil(t, repo)

	t.Cleanup(func() {
		_ = repo.Close()
	})

	repoImpl := repo.(*Repository[TestEntity])
	assert.Equal(t, "test", repoImpl.name)
	assert.NotNil(t, repoImpl.db)
}

func TestRepository_Create(t *testing.T) {
	repo := setupTestRepository(t)
	ctx := context.Background()

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

	entity := &TestEntity{
		ID:   uuid.New(),
		Name: "Test Entity",
		Age:  25,
	}
	created, err := repo.Create(ctx, entity)
	require.NoError(t, err)

	retrieved, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, retrieved.ID)
	assert.Equal(t, created.Name, retrieved.Name)
	assert.Equal(t, created.Age, retrieved.Age)

	nonExistentID := uuid.New()
	_, err = repo.GetByID(ctx, nonExistentID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRepository_Update(t *testing.T) {
	repo := setupTestRepository(t)
	ctx := context.Background()

	entity := &TestEntity{
		ID:   uuid.New(),
		Name: "Original Name",
		Age:  25,
	}
	created, err := repo.Create(ctx, entity)
	require.NoError(t, err)

	created.Name = "Updated Name"
	created.Age = 30

	updated, err := repo.Update(ctx, created)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", updated.Name)
	assert.Equal(t, 30, updated.Age)
	assert.Equal(t, created.ID, updated.ID)

	retrieved, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", retrieved.Name)
	assert.Equal(t, 30, retrieved.Age)
}

func TestRepository_Delete(t *testing.T) {
	repo := setupTestRepository(t)
	ctx := context.Background()

	entity := &TestEntity{
		ID:   uuid.New(),
		Name: "To Be Deleted",
		Age:  25,
	}
	created, err := repo.Create(ctx, entity)
	require.NoError(t, err)

	exists, err := repo.Exists(ctx, created.ID)
	require.NoError(t, err)
	assert.True(t, exists)

	err = repo.Delete(ctx, created.ID)
	require.NoError(t, err)

	exists, err = repo.Exists(ctx, created.ID)
	require.NoError(t, err)
	assert.False(t, exists)

	_, err = repo.GetByID(ctx, created.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRepository_Exists(t *testing.T) {
	repo := setupTestRepository(t)
	ctx := context.Background()

	nonExistentID := uuid.New()
	exists, err := repo.Exists(ctx, nonExistentID)
	require.NoError(t, err)
	assert.False(t, exists)

	entity := &TestEntity{
		ID:   uuid.New(),
		Name: "Test Entity",
		Age:  25,
	}
	created, err := repo.Create(ctx, entity)
	require.NoError(t, err)

	exists, err = repo.Exists(ctx, created.ID)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestRepository_Count(t *testing.T) {
	repo := setupTestRepository(t)
	ctx := context.Background()

	count, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	var createdIDs []uuid.UUID
	entities := []*TestEntity{
		{ID: uuid.New(), Name: "Entity 1", Age: 25},
		{ID: uuid.New(), Name: "Entity 2", Age: 30},
		{ID: uuid.New(), Name: "Entity 3", Age: 35},
	}

	for _, entity := range entities {
		created, err := repo.Create(ctx, entity)
		require.NoError(t, err)
		createdIDs = append(createdIDs, created.ID)
	}

	count, err = repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)

	err = repo.Delete(ctx, createdIDs[0])
	require.NoError(t, err)

	count, err = repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

func TestRepository_ContextCancellation(t *testing.T) {
	repo := setupTestRepository(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.GetByID(ctx, uuid.New())
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

	_, err = repo.Where(ctx, "1=1")
	assert.Error(t, err)
}

func setupTestRepository(t *testing.T) *Repository[TestEntity] {
	tempDir := t.TempDir()

	originalPath := os.Getenv("QUIVER_DATABASE_PATH")
	t.Cleanup(func() {
		if originalPath != "" {
			os.Setenv("QUIVER_DATABASE_PATH", originalPath)
		} else {
			os.Unsetenv("QUIVER_DATABASE_PATH")
		}
	})

	os.Setenv("QUIVER_DATABASE_PATH", tempDir)

	uniqueName := fmt.Sprintf("test_%s_%d", t.Name(), time.Now().UnixNano())
	repo, err := NewRepository[TestEntity](uniqueName)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = repo.Close()
	})

	return repo.(*Repository[TestEntity])
}

func TestRepository_ConcurrentOperations(t *testing.T) {
	repo := setupTestRepository(t)
	ctx := context.Background()

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

	for i := 0; i < numEntities; i++ {
		err := <-done
		require.NoError(t, err)
	}

	count, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(numEntities), count)

	entities, err := repo.Where(ctx, "1=1")
	require.NoError(t, err)
	assert.Len(t, entities, numEntities)
}

func TestRepository_Where(t *testing.T) {
	repo := setupTestRepository(t)
	ctx := context.Background()

	entities := []*TestEntity{
		{ID: uuid.New(), Name: "Entity 1", Age: 25},
		{ID: uuid.New(), Name: "Entity 2", Age: 30},
		{ID: uuid.New(), Name: "Entity 3", Age: 35},
	}

	for _, entity := range entities {
		_, err := repo.Create(ctx, entity)
		require.NoError(t, err)
	}

	allEntities, err := repo.Where(ctx, "1=1")
	require.NoError(t, err)
	assert.Len(t, allEntities, 3)

	filteredEntities, err := repo.Where(ctx, "age >= ?", 30)
	require.NoError(t, err)
	assert.Len(t, filteredEntities, 2)

	names := make(map[string]bool)
	for _, entity := range allEntities {
		names[entity.Name] = true
	}
	assert.True(t, names["Entity 1"])
	assert.True(t, names["Entity 2"])
	assert.True(t, names["Entity 3"])
}

func TestNewRepository_DirectoryCreationFailure(t *testing.T) {
	var invalidPath string

	if runtime.GOOS == "windows" {
		invalidPath = "C:\\CON\\invalid" // Windows file systems
	} else {
		invalidPath = "/dev/null/invalid" // Unix-like file systems (Linux + MacOS)
	}

	t.Setenv("QUIVER_DATABASE_PATH", invalidPath)

	repo, err := NewRepository[TestEntity]("test_failure")

	require.Error(t, err)
	assert.Nil(t, repo)
	assert.Contains(t, err.Error(), "failed to create database directory")
}

func TestNewRepository_ConnectionFailure(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	dbPath := filepath.Join(tempDir, "test_conn_fail.db")
	err := os.MkdirAll(dbPath, 0750)
	require.NoError(t, err)

	repo, err := NewRepository[TestEntity]("test_conn_fail")

	require.Error(t, err)
	assert.Nil(t, repo)
	assert.Contains(t, err.Error(), "failed to connect to database")
}

func TestNewRepository_MigrationFailure(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("QUIVER_DATABASE_PATH", tempDir)

	dbPath := filepath.Join(tempDir, "test_migration_fail.db")

	err := os.WriteFile(dbPath, []byte("not a valid sqlite database"), 0644)
	require.NoError(t, err)

	repo, err := NewRepository[TestEntity]("test_migration_fail")

	require.Error(t, err)
	assert.Nil(t, repo)
	assert.True(t,
		strings.Contains(err.Error(), "failed to connect to database") ||
			strings.Contains(err.Error(), "failed to migrate database schema"),
		"Expected connection or migration error, got: %v", err)
}

func TestRepository_Close_Success(t *testing.T) {
	tempDir := t.TempDir()

	originalPath := os.Getenv("QUIVER_DATABASE_PATH")
	t.Cleanup(func() {
		if originalPath != "" {
			os.Setenv("QUIVER_DATABASE_PATH", originalPath)
		} else {
			os.Unsetenv("QUIVER_DATABASE_PATH")
		}
	})

	os.Setenv("QUIVER_DATABASE_PATH", tempDir)

	repo, err := NewRepository[TestEntity]("test_close_success")
	require.NoError(t, err)
	require.NotNil(t, repo)

	ctx := context.Background()
	entity := &TestEntity{ID: uuid.New(), Name: "Test", Age: 25}
	_, err = repo.Create(ctx, entity)
	require.NoError(t, err)

	err = repo.Close()
	require.NoError(t, err)

	_, err = repo.GetByID(ctx, entity.ID)
	assert.Error(t, err)
}
