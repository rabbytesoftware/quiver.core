package eventsourcing

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	builder := New()
	assert.NotNil(t, builder)
}

func TestBuilder_WithSQLiteStore(t *testing.T) {
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

	ctx := context.Background()
	builder := New().WithSQLiteStore(ctx, "test")
	assert.NotNil(t, builder.eventStore)
	
	if builder.eventStore != nil {
		defer builder.eventStore.Close()
	}
}

func TestBuilder_WithMemoryBus(t *testing.T) {
	builder := New().WithMemoryBus()
	assert.NotNil(t, builder.eventBus)
}

func TestBuilder_Build_Success(t *testing.T) {
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

	ctx := context.Background()
	es, err := New().
		WithSQLiteStore(ctx, "test").
		WithMemoryBus().
		Build()

	require.NoError(t, err)
	assert.NotNil(t, es)
	defer es.Close()
}

func TestBuilder_Build_MissingStore(t *testing.T) {
	_, err := New().
		WithMemoryBus().
		Build()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "event store is required")
}

func TestBuilder_Build_MissingBus(t *testing.T) {
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

	ctx := context.Background()
	builder := New().WithSQLiteStore(ctx, "test")
	if builder.eventStore != nil {
		defer builder.eventStore.Close()
	}
	
	_, err := builder.Build()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "event bus is required")
}
