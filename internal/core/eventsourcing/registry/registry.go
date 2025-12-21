package registry

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

type Registry struct {
	mu     sync.RWMutex
	events map[string]reflect.Type
}

func NewRegistry() *Registry {
	return &Registry{
		events: make(map[string]reflect.Type),
	}
}

func (r *Registry) RegisterEvent(
	event domain.Event,
) {
	r.mu.Lock()
	defer r.mu.Unlock()

	eventType := event.GetEventType()
	eventValue := reflect.ValueOf(event)
	if eventValue.Kind() == reflect.Ptr {
		eventValue = eventValue.Elem()
	}
	r.events[eventType] = eventValue.Type()
}

func (r *Registry) GetEventType(
	eventType string,
) (reflect.Type, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	typ, exists := r.events[eventType]
	if !exists {
		return nil, fmt.Errorf("event type %s not registered", eventType)
	}

	return typ, nil
}
