package eventsourcing_test

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing"
	"github.com/stretchr/testify/assert"
)

func TestNewEventStream(t *testing.T) {
	events := []eventsourcing.Event{
		&SimpleTestEvent{
			BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "event.1"),
			Data:      "event 1",
		},
	}

	stream := eventsourcing.NewEventStream(events)
	assert.NotNil(t, stream)
	assert.Equal(t, 1, stream.Count())
}

func TestEventStream_All(t *testing.T) {
	events := []eventsourcing.Event{
		&SimpleTestEvent{BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "event.1")},
		&SimpleTestEvent{BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "event.2")},
	}

	stream := eventsourcing.NewEventStream(events)
	allEvents := stream.All()

	assert.Len(t, allEvents, 2)
	assert.Equal(t, events, allEvents)
}

func TestEventStream_Count(t *testing.T) {
	stream := eventsourcing.NewEventStream([]eventsourcing.Event{})
	assert.Equal(t, 0, stream.Count())

	events := []eventsourcing.Event{
		&SimpleTestEvent{BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "event.1")},
		&SimpleTestEvent{BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "event.2")},
		&SimpleTestEvent{BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "event.3")},
	}

	stream = eventsourcing.NewEventStream(events)
	assert.Equal(t, 3, stream.Count())
}

func TestEventStream_CountByType(t *testing.T) {
	events := []eventsourcing.Event{
		&SimpleTestEvent{BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "type.a")},
		&SimpleTestEvent{BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "type.a")},
		&SimpleTestEvent{BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "type.b")},
	}

	stream := eventsourcing.NewEventStream(events)

	assert.Equal(t, 2, stream.CountByType("type.a"))
	assert.Equal(t, 1, stream.CountByType("type.b"))
	assert.Equal(t, 0, stream.CountByType("type.c"))
}

func TestFindLast(t *testing.T) {
	events := []eventsourcing.Event{
		&SimpleTestEvent{BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "type.a"), Data: "first"},
		&SimpleTestEvent{BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "type.b"), Data: "middle"},
		&SimpleTestEvent{BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "type.a"), Data: "last"},
	}

	stream := eventsourcing.NewEventStream(events)

	last := eventsourcing.FindLast[SimpleTestEvent](stream, "type.a")
	assert.NotNil(t, last)
	assert.Equal(t, "last", last.Data)
}

func TestFindLast_NoMatch(t *testing.T) {
	events := []eventsourcing.Event{
		&SimpleTestEvent{BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "type.a")},
	}

	stream := eventsourcing.NewEventStream(events)

	result := eventsourcing.FindLast[SimpleTestEvent](stream, "type.nonexistent")
	assert.Nil(t, result)
}

func TestFindLast_EmptyStream(t *testing.T) {
	stream := eventsourcing.NewEventStream([]eventsourcing.Event{})

	result := eventsourcing.FindLast[SimpleTestEvent](stream, "type.a")
	assert.Nil(t, result)
}

func TestFilter(t *testing.T) {
	events := []eventsourcing.Event{
		&SimpleTestEvent{BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "type.a"), Data: "a1"},
		&SimpleTestEvent{BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "type.b"), Data: "b1"},
		&SimpleTestEvent{BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "type.a"), Data: "a2"},
		&SimpleTestEvent{BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "type.a"), Data: "a3"},
	}

	stream := eventsourcing.NewEventStream(events)

	filtered := eventsourcing.Filter[SimpleTestEvent](stream, "type.a")
	assert.Len(t, filtered, 3)
	assert.Equal(t, "a1", filtered[0].Data)
	assert.Equal(t, "a2", filtered[1].Data)
	assert.Equal(t, "a3", filtered[2].Data)
}

func TestFilter_NoMatch(t *testing.T) {
	events := []eventsourcing.Event{
		&SimpleTestEvent{BaseEvent: eventsourcing.NewBaseEvent("agg_1", "test", "type.a")},
	}

	stream := eventsourcing.NewEventStream(events)

	results := eventsourcing.Filter[SimpleTestEvent](stream, "type.nonexistent")
	assert.Empty(t, results)
}

func TestFilter_EmptyStream(t *testing.T) {
	stream := eventsourcing.NewEventStream([]eventsourcing.Event{})

	results := eventsourcing.Filter[SimpleTestEvent](stream, "type.a")
	assert.Empty(t, results)
}

