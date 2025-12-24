package commands

import (
	"context"
	"os"
	"testing"

	events "github.com/rabbytesoftware/quiver/internal/async/v1/arrow/events"
	eventsourcing "github.com/rabbytesoftware/quiver/internal/core/es"
	"github.com/rabbytesoftware/quiver/internal/models/arrow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAddRequest(t *testing.T) {
	req := NewAddRequest("test-namespace", "/path/to/arrow", true)
	require.NotNil(t, req)
}

func TestAddRequest_ToEvent(t *testing.T) {
	req := NewAddRequest("test-namespace", "/path/to/arrow", false)

	evt, err := req.ToEvent()
	require.NoError(t, err)
	require.NotNil(t, evt)
	assert.Equal(t, "test-namespace", evt.Payload.ArrowNamespace)
	assert.Equal(t, "/path/to/arrow", evt.Payload.Path)
	assert.False(t, evt.Payload.ForceAdd)
	assert.Equal(t, int64(1), evt.Payload.Step)
}

func TestAddRequest_Validate_Success(t *testing.T) {
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
	es, err := eventsourcing.New().
		WithSQLiteStore(ctx, "add_request_test").
		WithMemoryBus().
		Build()
	require.NoError(t, err)

	req := NewAddRequest("test-namespace", "/path/to/arrow", false)
	err = req.Validate(ctx, es)
	assert.NoError(t, err)
}

func TestAddRequest_Validate_AlreadyExists(t *testing.T) {
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
	es, err := eventsourcing.New().
		WithSQLiteStore(ctx, "add_request_exists_test").
		WithMemoryBus().
		Build()
	require.NoError(t, err)

	testArrow := &arrow.Arrow{
		Name:    "test-arrow",
		Version: "1.0.0",
	}

	succeededEvt, err := events.NewAddSucceededEvent().
		WithArrowNamespace("test-namespace").
		WithArrow(testArrow).
		WithStep(1).
		Build()
	require.NoError(t, err)

	err = eventsourcing.Publish(es, ctx, succeededEvt)
	require.NoError(t, err)

	req := NewAddRequest("test-namespace", "/path/to/arrow", false)
	err = req.Validate(ctx, es)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ARROW_ALREADY_EXISTS")
}
