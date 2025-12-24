package eventsourcing

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/core/es/command"
	"github.com/rabbytesoftware/quiver/internal/core/es/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestPayload struct {
	Data string `json:"data"`
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

	handlerCalled := false
	err = es.Subscribe("test.Event", func(ctx context.Context, evt *event.Event[any]) error {
		handlerCalled = true
		return nil
	})
	require.NoError(t, err)

	payload := &TestPayload{Data: "test data"}
	evt, err := event.NewEvent[TestPayload]().
		WithAggregateID("agg1").
		WithAggregateType("test").
		WithAggregateVersion(1).
		WithEventType("test.Event").
		WithEventVersion("v1").
		WithPayload(payload).
		Build()
	require.NoError(t, err)

	err = Publish(es, ctx, evt)
	require.NoError(t, err)
	assert.True(t, handlerCalled)

	exists, err := es.AggregateExists(ctx, "agg1")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestEventSourcing_GetEvents(t *testing.T) {
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
		WithSQLiteStore(ctx, "get_events_test").
		WithMemoryBus().
		Build()

	require.NoError(t, err)
	require.NotNil(t, es)

	payload1 := &TestPayload{Data: "event 1"}
	evt1, err := event.NewEvent[TestPayload]().
		WithAggregateID("agg1").
		WithAggregateType("test").
		WithAggregateVersion(1).
		WithEventType("test.Event").
		WithEventVersion("v1").
		WithPayload(payload1).
		Build()
	require.NoError(t, err)

	err = Publish(es, ctx, evt1)
	require.NoError(t, err)

	payload2 := &TestPayload{Data: "event 2"}
	evt2, err := event.NewEvent[TestPayload]().
		WithAggregateID("agg1").
		WithAggregateType("test").
		WithAggregateVersion(2).
		WithEventType("test.Event").
		WithEventVersion("v1").
		WithPayload(payload2).
		Build()
	require.NoError(t, err)

	err = Publish(es, ctx, evt2)
	require.NoError(t, err)

	events, err := GetEvents[any](es, ctx, "agg1")
	require.NoError(t, err)
	assert.Len(t, events, 2)
}

func TestEventSourcing_GetEventsTyped(t *testing.T) {
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
		WithSQLiteStore(ctx, "get_events_typed_test").
		WithMemoryBus().
		Build()

	require.NoError(t, err)
	require.NotNil(t, es)

	payload := &TestPayload{Data: "test data"}
	evt, err := event.NewEvent[TestPayload]().
		WithAggregateID("agg1").
		WithAggregateType("test").
		WithAggregateVersion(1).
		WithEventType("test.Event").
		WithEventVersion("v1").
		WithPayload(payload).
		Build()
	require.NoError(t, err)

	err = Publish(es, ctx, evt)
	require.NoError(t, err)

	events, err := GetEvents[TestPayload](es, ctx, "agg1")
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "test data", events[0].Payload.Data)
}

func TestEventSourcing_HasEventType(t *testing.T) {
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
		WithSQLiteStore(ctx, "has_event_type_test").
		WithMemoryBus().
		Build()

	require.NoError(t, err)
	require.NotNil(t, es)

	has, err := es.HasEventType(ctx, "agg1", "test.Event")
	require.NoError(t, err)
	assert.False(t, has)

	payload := &TestPayload{Data: "test data"}
	evt, err := event.NewEvent[TestPayload]().
		WithAggregateID("agg1").
		WithAggregateType("test").
		WithAggregateVersion(1).
		WithEventType("test.Event").
		WithEventVersion("v1").
		WithPayload(payload).
		Build()
	require.NoError(t, err)

	err = Publish(es, ctx, evt)
	require.NoError(t, err)

	has, err = es.HasEventType(ctx, "agg1", "test.Event")
	require.NoError(t, err)
	assert.True(t, has)
}

func TestEventSourcing_GetEventStream(t *testing.T) {
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
		WithSQLiteStore(ctx, "get_event_stream_test").
		WithMemoryBus().
		Build()

	require.NoError(t, err)
	require.NotNil(t, es)

	payload := &TestPayload{Data: "test data"}
	evt, err := event.NewEvent[TestPayload]().
		WithAggregateID("agg1").
		WithAggregateType("test").
		WithAggregateVersion(1).
		WithEventType("test.Event").
		WithEventVersion("v1").
		WithPayload(payload).
		Build()
	require.NoError(t, err)

	err = Publish(es, ctx, evt)
	require.NoError(t, err)

	stream, err := GetEventStream[any](es, ctx, "agg1")
	require.NoError(t, err)
	assert.Equal(t, 1, stream.Len())
	assert.True(t, stream.HasEventType("test.Event"))
}

type TestCommand struct {
	aggregateID string
	data        string
	shouldFail  bool
}

func (c *TestCommand) Validate(ctx context.Context, es command.EventSourcingValidator) error {
	if c.shouldFail {
		return fmt.Errorf("validation failed")
	}
	return nil
}

func (c *TestCommand) ToEvent() (*event.Event[TestPayload], error) {
	return event.NewEvent[TestPayload]().
		WithAggregateID(c.aggregateID).
		WithAggregateType("test").
		WithAggregateVersion(1).
		WithEventType("test.Event").
		WithEventVersion("v1").
		WithPayload(&TestPayload{Data: c.data}).
		Build()
}

func TestEventSourcing_Execute_Success(t *testing.T) {
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
		WithSQLiteStore(ctx, "execute_test").
		WithMemoryBus().
		Build()

	require.NoError(t, err)
	require.NotNil(t, es)

	cmd := &TestCommand{
		aggregateID: "agg1",
		data:        "test data",
		shouldFail:  false,
	}

	err = Execute(es, ctx, cmd)
	require.NoError(t, err)

	exists, err := es.AggregateExists(ctx, "agg1")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestEventSourcing_Execute_ValidationFailed(t *testing.T) {
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
		WithSQLiteStore(ctx, "execute_validation_test").
		WithMemoryBus().
		Build()

	require.NoError(t, err)
	require.NotNil(t, es)

	cmd := &TestCommand{
		aggregateID: "agg1",
		data:        "test data",
		shouldFail:  true,
	}

	err = Execute(es, ctx, cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}
