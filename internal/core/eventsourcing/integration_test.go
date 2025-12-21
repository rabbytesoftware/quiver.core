package eventsourcing

import (
	"context"
	"os"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type integrationTestEvent struct {
	domain.BaseEvent
	Data string `json:"data"`
}

func (e *integrationTestEvent) GetEventType() string {
	return "test.Event"
}

func TestEventSourcing_Integration(t *testing.T) {
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
		WithSQLiteStore(ctx, "integration_test").
		WithMemoryBus().
		Build()

	require.NoError(t, err)
	require.NotNil(t, es)

	event := &integrationTestEvent{
		BaseEvent: domain.NewBaseEvent("agg1", "test", "test.Event"),
		Data:      "test data",
	}
	es.RegisterEvent(event)

	handlerCalled := false
	err = es.Subscribe("test.Event", func(ctx context.Context, evt domain.Event) error {
		handlerCalled = true
		return nil
	})
	require.NoError(t, err)

	cmd := NewCommand("agg1").
		WithEventFactory(func(version int64, metadata map[string]interface{}) domain.Event {
			return &integrationTestEvent{
				BaseEvent: domain.NewBaseEvent("agg1", "test", "test.Event"),
				Data:      "test data",
			}
		}).
		RequireNotExists().
		Build()

	err = es.ExecuteCommand(ctx, cmd)
	require.NoError(t, err)
	assert.True(t, handlerCalled)

	exists, err := es.AggregateExists(ctx, "agg1")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestEventSourcing_Idempotency(t *testing.T) {
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
		WithSQLiteStore(ctx, "idempotency_test_unique").
		WithMemoryBus().
		Build()

	require.NoError(t, err)
	require.NotNil(t, es)

	event := &integrationTestEvent{
		BaseEvent: domain.NewBaseEvent("agg2", "test", "test.Event"),
		Data:      "test data",
	}
	es.RegisterEvent(event)

	cmd1 := NewCommand("agg2").
		WithEventFactory(func(version int64, metadata map[string]interface{}) domain.Event {
			return &integrationTestEvent{
				BaseEvent: domain.NewBaseEvent("agg2", "test", "test.Event"),
				Data:      "test data",
			}
		}).
		RequireNotExists().
		Build()

	err = es.ExecuteCommand(ctx, cmd1)
	require.NoError(t, err)

	exists1, err := es.AggregateExists(ctx, "agg2")
	require.NoError(t, err)
	assert.True(t, exists1)

	cmd2 := NewCommand("agg2").
		WithEventFactory(func(version int64, metadata map[string]interface{}) domain.Event {
			return &integrationTestEvent{
				BaseEvent: domain.NewBaseEvent("agg2", "test", "test.Event"),
				Data:      "test data",
			}
		}).
		RequireExists().
		Build()

	err = es.ExecuteCommand(ctx, cmd2)
	require.NoError(t, err)

	exists2, err := es.AggregateExists(ctx, "agg2")
	require.NoError(t, err)
	assert.True(t, exists2)
}

func TestEventSourcing_ValidationHelpers(t *testing.T) {
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
		WithSQLiteStore(ctx, "validation_test").
		WithMemoryBus().
		Build()

	require.NoError(t, err)
	require.NotNil(t, es)

	exists, err := es.AggregateExists(ctx, "nonexistent")
	require.NoError(t, err)
	assert.False(t, exists)

	event := &integrationTestEvent{
		BaseEvent: domain.NewBaseEvent("agg1", "test", "test.Event"),
		Data:      "test data",
	}
	es.RegisterEvent(event)

	cmd := NewCommand("agg1").
		WithEventFactory(func(version int64, metadata map[string]interface{}) domain.Event {
			return &integrationTestEvent{
				BaseEvent: domain.NewBaseEvent("agg1", "test", "test.Event"),
				Data:      "test data",
			}
		}).
		RequireNotExists().
		Build()

	err = es.ExecuteCommand(ctx, cmd)
	require.NoError(t, err)

	exists, err = es.AggregateExists(ctx, "agg1")
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = es.AggregateExists(ctx, "agg1")
	require.NoError(t, err)
	assert.True(t, exists)
}

