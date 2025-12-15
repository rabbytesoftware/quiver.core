package internal

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

type EventFactory func() domain.Event

type DeserializeFunc func(eventType string, data []byte) (domain.Event, error)

type Registry struct {
	factories map[string]EventFactory
	mu        sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]EventFactory),
	}
}

func (r *Registry) Register(
	event domain.Event,
) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var eventType string
	eventValue := reflect.ValueOf(event)
	if eventValue.Kind() == reflect.Ptr {
		eventValue = eventValue.Elem()
	}

	eventTypeMethod := eventValue.MethodByName("EventType")
	if eventTypeMethod.IsValid() && eventTypeMethod.Type().NumIn() == 0 && eventTypeMethod.Type().NumOut() == 1 {
		result := eventTypeMethod.Call(nil)
		if len(result) > 0 && result[0].Kind() == reflect.String {
			eventType = result[0].String()
		}
	}

	if eventType == "" {
		eventType = event.GetEventType()
	}

	typ := reflect.TypeOf(event).Elem()

	r.factories[eventType] = func() domain.Event {
		return reflect.New(typ).Interface().(domain.Event)
	}
}

func (r *Registry) Deserialize(eventType string, data []byte) (domain.Event, error) {
	r.mu.RLock()
	factory, ok := r.factories[eventType]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown event type: %s", eventType)
	}

	event := factory()
	if err := json.Unmarshal(data, event); err != nil {
		return nil, fmt.Errorf("failed to deserialize event: %w", err)
	}

	return event, nil
}
