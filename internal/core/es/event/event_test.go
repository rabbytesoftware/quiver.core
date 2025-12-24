package event

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestPayload struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}

func TestNewEvent(t *testing.T) {
	builder := NewEvent[TestPayload]()
	require.NotNil(t, builder)
}

func TestEvent_Build_Success(t *testing.T) {
	payload := &TestPayload{
		Message: "test message",
		Count:   42,
	}

	evt, err := NewEvent[TestPayload]().
		WithAggregateID("agg-123").
		WithAggregateType("TestAggregate").
		WithAggregateVersion(1).
		WithEventType("test.created").
		WithEventVersion("v1").
		WithPayload(payload).
		Build()

	require.NoError(t, err)
	require.NotNil(t, evt)
	assert.NotEmpty(t, evt.EventID)
	assert.Equal(t, "agg-123", evt.AggregateID)
	assert.Equal(t, "TestAggregate", evt.AggregateType)
	assert.Equal(t, int64(1), evt.AggregateVersion)
	assert.Equal(t, "test.created", evt.EventType)
	assert.Equal(t, "v1", evt.EventVersion)
	assert.True(t, evt.Idempotency)
	assert.NotNil(t, evt.Payload)
	assert.Equal(t, "test message", evt.Payload.Message)
	assert.Equal(t, 42, evt.Payload.Count)
	assert.False(t, evt.Timestamp.IsZero())
}

func TestEvent_Build_WithMetadata(t *testing.T) {
	payload := &TestPayload{Message: "test"}
	metadata := map[string]string{
		"user":   "admin",
		"source": "api",
	}

	evt, err := NewEvent[TestPayload]().
		WithAggregateID("agg-123").
		WithAggregateType("TestAggregate").
		WithAggregateVersion(1).
		WithEventType("test.created").
		WithEventVersion("v1").
		WithMetadata(metadata).
		WithPayload(payload).
		Build()

	require.NoError(t, err)
	require.NotNil(t, evt)
	assert.Equal(t, "admin", evt.Metadata["user"])
	assert.Equal(t, "api", evt.Metadata["source"])
}

func TestEvent_Build_WithParentID(t *testing.T) {
	payload := &TestPayload{Message: "test"}
	parentID := "parent-123"

	evt, err := NewEvent[TestPayload]().
		WithAggregateID("agg-123").
		WithAggregateType("TestAggregate").
		WithAggregateVersion(1).
		WithEventType("test.created").
		WithEventVersion("v1").
		WithParentID(&parentID).
		WithPayload(payload).
		Build()

	require.NoError(t, err)
	require.NotNil(t, evt)
	require.NotNil(t, evt.ParentID)
	assert.Equal(t, "parent-123", *evt.ParentID)
}

func TestEvent_Build_WithIdempotency(t *testing.T) {
	payload := &TestPayload{Message: "test"}

	evt, err := NewEvent[TestPayload]().
		WithAggregateID("agg-123").
		WithAggregateType("TestAggregate").
		WithAggregateVersion(1).
		WithEventType("test.created").
		WithEventVersion("v1").
		WithIdempotency(true).
		WithPayload(payload).
		Build()

	require.NoError(t, err)
	require.NotNil(t, evt)
	assert.True(t, evt.Idempotency)
}

func TestEvent_Build_MissingAggregateID(t *testing.T) {
	payload := &TestPayload{Message: "test"}

	evt, err := NewEvent[TestPayload]().
		WithAggregateType("TestAggregate").
		WithAggregateVersion(1).
		WithEventType("test.created").
		WithEventVersion("v1").
		WithPayload(payload).
		Build()

	assert.Error(t, err)
	assert.Nil(t, evt)
	assert.Contains(t, err.Error(), "invalid event")
}

func TestEvent_Build_MissingAggregateType(t *testing.T) {
	payload := &TestPayload{Message: "test"}

	evt, err := NewEvent[TestPayload]().
		WithAggregateID("agg-123").
		WithAggregateVersion(1).
		WithEventType("test.created").
		WithEventVersion("v1").
		WithPayload(payload).
		Build()

	assert.Error(t, err)
	assert.Nil(t, evt)
}

func TestEvent_Build_MissingEventType(t *testing.T) {
	payload := &TestPayload{Message: "test"}

	evt, err := NewEvent[TestPayload]().
		WithAggregateID("agg-123").
		WithAggregateType("TestAggregate").
		WithAggregateVersion(1).
		WithEventVersion("v1").
		WithPayload(payload).
		Build()

	assert.Error(t, err)
	assert.Nil(t, evt)
}

func TestEvent_Build_MissingPayload(t *testing.T) {
	evt, err := NewEvent[TestPayload]().
		WithAggregateID("agg-123").
		WithAggregateType("TestAggregate").
		WithAggregateVersion(1).
		WithEventType("test.created").
		WithEventVersion("v1").
		Build()

	assert.Error(t, err)
	assert.Nil(t, evt)
}

func TestEvent_Builder_Chaining(t *testing.T) {
	payload := &TestPayload{Message: "test"}

	builder := NewEvent[TestPayload]()
	builder = builder.WithAggregateID("agg-123")
	builder = builder.WithAggregateType("TestAggregate")
	builder = builder.WithAggregateVersion(1)
	builder = builder.WithEventType("test.created")
	builder = builder.WithEventVersion("v1")
	builder = builder.WithPayload(payload)

	evt, err := builder.Build()

	require.NoError(t, err)
	require.NotNil(t, evt)
	assert.Equal(t, "agg-123", evt.AggregateID)
}
