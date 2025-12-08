package eventsourcing

import (
	"encoding/json"
	"fmt"
)

type EventFactory[T Event] func() T

type Registry[T Event] struct {
	factories map[string]EventFactory[T]
}

func NewRegistry[T Event]() *Registry[T] {
	return &Registry[T]{
		factories: make(map[string]EventFactory[T]),
	}
}

func (r *Registry[T]) Register(eventType string, factory EventFactory[T]) {
	r.factories[eventType] = factory
}

func (r *Registry[T]) Deserialize(eventType string, data []byte) (T, error) {
	factory, ok := r.factories[eventType]
	if !ok {
		var zero T
		return zero, fmt.Errorf("unknown event type: %s", eventType)
	}

	event := factory()
	if err := json.Unmarshal(data, event); err != nil {
		var zero T
		return zero, fmt.Errorf("failed to deserialize event: %w", err)
	}

	return event, nil
}
