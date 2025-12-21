package eventsourcing

import (
	"context"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testEvent struct {
	domain.BaseEvent
	Data string `json:"data"`
}

func (e *testEvent) GetEventType() string {
	return "test.Event"
}

func TestNewCommand(t *testing.T) {
	cmd := NewCommand("test-aggregate")
	assert.NotNil(t, cmd)
	assert.Equal(t, "test-aggregate", cmd.aggregateID)
}

func TestCommandBuilder_WithEventFactory(t *testing.T) {
	cmd := NewCommand("test-aggregate").
		WithEventFactory(func(version int64, metadata map[string]interface{}) domain.Event {
			return &testEvent{
				BaseEvent: domain.NewBaseEvent("test-aggregate", "test", "test.Event"),
				Data:      "test data",
			}
		})

	assert.NotNil(t, cmd.eventFactory)
}

func TestCommandBuilder_WithClientIP(t *testing.T) {
	cmd := NewCommand("test-aggregate").WithClientIP("127.0.0.1")
	assert.Equal(t, "127.0.0.1", cmd.clientIP)
}

func TestCommandBuilder_WithIdempotencyKey(t *testing.T) {
	cmd := NewCommand("test-aggregate").WithIdempotencyKey("key-123")
	assert.Equal(t, "key-123", cmd.idempotencyKey)
}

func TestCommandBuilder_RequireExists(t *testing.T) {
	cmd := NewCommand("test-aggregate").RequireExists()
	assert.True(t, cmd.requireExists)
}

func TestCommandBuilder_RequireNotExists(t *testing.T) {
	cmd := NewCommand("test-aggregate").RequireNotExists()
	assert.True(t, cmd.requireNotExists)
}

func TestCommandBuilder_RequireSucceeded(t *testing.T) {
	cmd := NewCommand("test-aggregate").RequireSucceeded("test.Event")
	assert.Equal(t, "test.Event", cmd.requireSucceeded)
}

func TestCommand_GetAggregateID(t *testing.T) {
	cmd := NewCommand("test-aggregate").Build()
	assert.Equal(t, "test-aggregate", cmd.GetAggregateID())
}

func TestCommand_ToRequestedEvent(t *testing.T) {
	cmd := NewCommand("test-aggregate").
		WithEventFactory(func(version int64, metadata map[string]interface{}) domain.Event {
			return &testEvent{
				BaseEvent: domain.NewBaseEventWithMetadata(
					"test-aggregate",
					"test",
					"test.Event",
					version,
					"",
					nil,
					metadata,
				),
				Data: "test data",
			}
		}).
		WithClientIP("127.0.0.1").
		Build()

	event := cmd.ToRequestedEvent()
	require.NotNil(t, event)
	assert.Equal(t, "test-aggregate", event.GetAggregateID())
	assert.Equal(t, "127.0.0.1", event.GetMetadata()["client_ip"])
}

func TestCommand_Validate_RequireExists(t *testing.T) {
	mockES := &mockEventSourcingValidator{
		exists: true,
	}

	cmd := NewCommand("test-aggregate").
		RequireExists().
		Build()

	ctx := context.Background()
	err := cmd.Validate(ctx, mockES)
	assert.NoError(t, err)
}

func TestCommand_Validate_RequireExists_Fails(t *testing.T) {
	mockES := &mockEventSourcingValidator{
		exists: false,
	}

	cmd := NewCommand("test-aggregate").
		RequireExists().
		Build()

	ctx := context.Background()
	err := cmd.Validate(ctx, mockES)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

func TestCommand_Validate_RequireNotExists(t *testing.T) {
	mockES := &mockEventSourcingValidator{
		exists: false,
	}

	cmd := NewCommand("test-aggregate").
		RequireNotExists().
		Build()

	ctx := context.Background()
	err := cmd.Validate(ctx, mockES)
	assert.NoError(t, err)
}

func TestCommand_Validate_RequireNotExists_Fails(t *testing.T) {
	mockES := &mockEventSourcingValidator{
		exists: true,
	}

	cmd := NewCommand("test-aggregate").
		RequireNotExists().
		Build()

	ctx := context.Background()
	err := cmd.Validate(ctx, mockES)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestCommand_Validate_RequireSucceeded(t *testing.T) {
	mockES := &mockEventSourcingValidator{
		hasSucceeded: true,
	}

	cmd := NewCommand("test-aggregate").
		RequireSucceeded("test.Event").
		Build()

	ctx := context.Background()
	err := cmd.Validate(ctx, mockES)
	assert.NoError(t, err)
}

func TestCommand_Validate_RequireSucceeded_Fails(t *testing.T) {
	mockES := &mockEventSourcingValidator{
		hasSucceeded: false,
	}

	cmd := NewCommand("test-aggregate").
		RequireSucceeded("test.Event").
		Build()

	ctx := context.Background()
	err := cmd.Validate(ctx, mockES)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

type mockEventSourcingValidator struct {
	exists       bool
	hasSucceeded bool
}

func (m *mockEventSourcingValidator) AggregateExists(ctx context.Context, aggregateID string) (bool, error) {
	return m.exists, nil
}

func (m *mockEventSourcingValidator) HasSucceededEvent(ctx context.Context, aggregateID string, eventTypePrefix string) (bool, error) {
	return m.hasSucceeded, nil
}

func (m *mockEventSourcingValidator) GetEvents(ctx context.Context, aggregateID string) (domain.EventStream, error) {
	return nil, nil
}
