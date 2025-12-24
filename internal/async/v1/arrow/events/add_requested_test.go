package events

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/models/arrow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAddRequestedEvent(t *testing.T) {
	event := NewAddRequestedEvent()
	require.NotNil(t, event)
}

func TestAddRequestedEvent_Build(t *testing.T) {
	evt, err := NewAddRequestedEvent().
		WithArrowNamespace("test-namespace").
		WithPath("/path/to/arrow").
		WithForceAdd(true).
		WithStep(1).
		Build()

	require.NoError(t, err)
	require.NotNil(t, evt)
	assert.Equal(t, "test-namespace", evt.AggregateID)
	assert.Equal(t, ArrowAggregateType, evt.AggregateType)
	assert.Equal(t, int64(1), evt.AggregateVersion)
	assert.Equal(t, ArrowAddArrowRequestedType, evt.EventType)
	assert.NotNil(t, evt.Payload)
	assert.Equal(t, "test-namespace", evt.Payload.ArrowNamespace)
	assert.Equal(t, "/path/to/arrow", evt.Payload.Path)
	assert.True(t, evt.Payload.ForceAdd)
	assert.Equal(t, int64(1), evt.Payload.Step)
}

func TestAddRequestedEvent_Build_MissingFields(t *testing.T) {
	evt, err := NewAddRequestedEvent().
		WithArrowNamespace("").
		Build()

	assert.Error(t, err)
	assert.Nil(t, evt)
}

func TestAddSucceededEvent_Build(t *testing.T) {
	testArrow := &arrow.Arrow{
		Name:    "test-arrow",
		Version: "1.0.0",
	}

	evt, err := NewAddSucceededEvent().
		WithArrowNamespace("test-namespace").
		WithArrow(testArrow).
		WithStep(2).
		Build()

	require.NoError(t, err)
	require.NotNil(t, evt)
	assert.Equal(t, "test-namespace", evt.AggregateID)
	assert.Equal(t, ArrowAggregateType, evt.AggregateType)
	assert.Equal(t, int64(2), evt.AggregateVersion)
	assert.Equal(t, ArrowAddArrowSucceededType, evt.EventType)
	assert.NotNil(t, evt.Payload)
	assert.Equal(t, "test-namespace", evt.Payload.ArrowNamespace)
	assert.Equal(t, testArrow, evt.Payload.Arrow)
	assert.Equal(t, int64(2), evt.Payload.Step)
}

func TestAddSucceededEvent_Build_MissingFields(t *testing.T) {
	evt, err := NewAddSucceededEvent().
		WithArrowNamespace("test-namespace").
		Build()

	assert.Error(t, err)
	assert.Nil(t, evt)
}

func TestAddFailedEvent_Build(t *testing.T) {
	evt, err := NewAddFailedEvent().
		WithArrowNamespace("test-namespace").
		WithErrorCode("ERR_001").
		WithErrorMessage("Failed to add arrow").
		WithStep(3).
		Build()

	require.NoError(t, err)
	require.NotNil(t, evt)
	assert.Equal(t, "test-namespace", evt.AggregateID)
	assert.Equal(t, ArrowAggregateType, evt.AggregateType)
	assert.Equal(t, int64(3), evt.AggregateVersion)
	assert.Equal(t, ArrowAddArrowFailedType, evt.EventType)
	assert.NotNil(t, evt.Payload)
	assert.Equal(t, "test-namespace", evt.Payload.ArrowNamespace)
	assert.Equal(t, "ERR_001", evt.Payload.Error.Code)
	assert.Equal(t, "Failed to add arrow", evt.Payload.Error.Message)
	assert.Equal(t, int64(3), evt.Payload.Step)
}

func TestAddFailedEvent_Build_MissingFields(t *testing.T) {
	evt, err := NewAddFailedEvent().
		WithArrowNamespace("test-namespace").
		WithErrorCode("ERR_001").
		Build()

	assert.Error(t, err)
	assert.Nil(t, evt)
}
