package stream

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/core/es/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestPayload struct {
	Data string `json:"data"`
}

func TestNewEventStream(t *testing.T) {
	events := []*event.Event[TestPayload]{
		{EventType: "test.event1"},
		{EventType: "test.event2"},
	}

	stream := NewEventStream(events)
	require.NotNil(t, stream)
	assert.Len(t, stream.events, 2)
}

func TestEventStream_HasEventType_True(t *testing.T) {
	events := []*event.Event[TestPayload]{
		{EventType: "test.event1"},
		{EventType: "test.event2"},
		{EventType: "test.event3"},
	}

	stream := NewEventStream(events)
	assert.True(t, stream.HasEventType("test.event2"))
}

func TestEventStream_HasEventType_False(t *testing.T) {
	events := []*event.Event[TestPayload]{
		{EventType: "test.event1"},
		{EventType: "test.event2"},
	}

	stream := NewEventStream(events)
	assert.False(t, stream.HasEventType("test.event3"))
}

func TestEventStream_HasEventType_EmptyStream(t *testing.T) {
	events := []*event.Event[TestPayload]{}

	stream := NewEventStream(events)
	assert.False(t, stream.HasEventType("test.event"))
}

func TestEventStream_GetEvents(t *testing.T) {
	events := []*event.Event[TestPayload]{
		{EventType: "test.event1"},
		{EventType: "test.event2"},
	}

	stream := NewEventStream(events)
	retrieved := stream.GetEvents()

	assert.Equal(t, events, retrieved)
	assert.Len(t, retrieved, 2)
}

func TestEventStream_Len(t *testing.T) {
	events := []*event.Event[TestPayload]{
		{EventType: "test.event1"},
		{EventType: "test.event2"},
		{EventType: "test.event3"},
	}

	stream := NewEventStream(events)
	assert.Equal(t, 3, stream.Len())
}

func TestEventStream_Len_Empty(t *testing.T) {
	events := []*event.Event[TestPayload]{}

	stream := NewEventStream(events)
	assert.Equal(t, 0, stream.Len())
}

func TestEventStream_GetLatestVersion(t *testing.T) {
	events := []*event.Event[TestPayload]{
		{EventType: "test.event1", AggregateVersion: 1},
		{EventType: "test.event2", AggregateVersion: 3},
		{EventType: "test.event3", AggregateVersion: 2},
	}

	stream := NewEventStream(events)
	assert.Equal(t, int64(3), stream.GetLatestVersion())
}

func TestEventStream_GetLatestVersion_SingleEvent(t *testing.T) {
	events := []*event.Event[TestPayload]{
		{EventType: "test.event1", AggregateVersion: 5},
	}

	stream := NewEventStream(events)
	assert.Equal(t, int64(5), stream.GetLatestVersion())
}

func TestEventStream_GetLatestVersion_Empty(t *testing.T) {
	events := []*event.Event[TestPayload]{}

	stream := NewEventStream(events)
	assert.Equal(t, int64(0), stream.GetLatestVersion())
}

func TestEventStream_GetLatestVersion_AllZero(t *testing.T) {
	events := []*event.Event[TestPayload]{
		{EventType: "test.event1", AggregateVersion: 0},
		{EventType: "test.event2", AggregateVersion: 0},
	}

	stream := NewEventStream(events)
	assert.Equal(t, int64(0), stream.GetLatestVersion())
}

func TestEventStream_Operations(t *testing.T) {
	events := []*event.Event[TestPayload]{
		{EventType: "arrow.created", AggregateVersion: 1},
		{EventType: "arrow.updated", AggregateVersion: 2},
		{EventType: "arrow.deleted", AggregateVersion: 3},
	}

	stream := NewEventStream(events)

	assert.Equal(t, 3, stream.Len())
	assert.True(t, stream.HasEventType("arrow.created"))
	assert.True(t, stream.HasEventType("arrow.updated"))
	assert.True(t, stream.HasEventType("arrow.deleted"))
	assert.False(t, stream.HasEventType("arrow.archived"))
	assert.Equal(t, int64(3), stream.GetLatestVersion())
}
