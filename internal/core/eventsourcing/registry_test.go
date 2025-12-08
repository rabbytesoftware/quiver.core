package eventsourcing_test

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing"
	"github.com/stretchr/testify/assert"
)

type SimpleTestEvent struct {
	eventsourcing.BaseEvent
	Data string `json:"data"`
}

func TestNewRegistry(t *testing.T) {
	registry := eventsourcing.NewRegistry[eventsourcing.Event]()
	assert.NotNil(t, registry)
}

func TestRegistry_Register(t *testing.T) {
	registry := eventsourcing.NewRegistry[eventsourcing.Event]()

	registry.Register("simple.event", func() eventsourcing.Event {
		return &SimpleTestEvent{}
	})

	jsonData := []byte(`{"id":"123","aggregate_id":"agg","aggregate_type":"test","event_type":"simple.event","version":1,"timestamp":"2024-01-01T00:00:00Z","data":"test"}`)
	event, err := registry.Deserialize("simple.event", jsonData)

	assert.NoError(t, err)
	assert.NotNil(t, event)

	simpleEvent := event.(*SimpleTestEvent)
	assert.Equal(t, "test", simpleEvent.Data)
}

func TestRegistry_Deserialize_UnknownType(t *testing.T) {
	registry := eventsourcing.NewRegistry[eventsourcing.Event]()

	_, err := registry.Deserialize("unknown.event", []byte(`{}`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown event type")
}

func TestRegistry_Deserialize_InvalidJSON(t *testing.T) {
	registry := eventsourcing.NewRegistry[eventsourcing.Event]()
	registry.Register("test.event", func() eventsourcing.Event {
		return &SimpleTestEvent{}
	})

	_, err := registry.Deserialize("test.event", []byte(`{invalid json`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to deserialize event")
}

func TestRegistry_Deserialize_Success(t *testing.T) {
	registry := eventsourcing.NewRegistry[eventsourcing.Event]()
	registry.Register("test.event", func() eventsourcing.Event {
		return &SimpleTestEvent{}
	})

	jsonData := []byte(`{"data":"hello world"}`)
	event, err := registry.Deserialize("test.event", jsonData)

	assert.NoError(t, err)
	assert.NotNil(t, event)

	simpleEvent := event.(*SimpleTestEvent)
	assert.Equal(t, "hello world", simpleEvent.Data)
}

func TestRegistry_MultipleEventTypes(t *testing.T) {
	registry := eventsourcing.NewRegistry[eventsourcing.Event]()

	registry.Register("event.type1", func() eventsourcing.Event {
		return &SimpleTestEvent{}
	})
	registry.Register("event.type2", func() eventsourcing.Event {
		return &SimpleTestEvent{}
	})

	event1, err := registry.Deserialize("event.type1", []byte(`{"data":"type1"}`))
	assert.NoError(t, err)
	assert.Equal(t, "type1", event1.(*SimpleTestEvent).Data)

	event2, err := registry.Deserialize("event.type2", []byte(`{"data":"type2"}`))
	assert.NoError(t, err)
	assert.Equal(t, "type2", event2.(*SimpleTestEvent).Data)
}

