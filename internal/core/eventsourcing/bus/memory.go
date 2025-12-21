package bus

import (
	"context"
	"fmt"
	"regexp"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

type regexSubscription struct {
	pattern *regexp.Regexp
	handler EventHandler
	raw     string
}

type MemoryBus struct {
	mu           sync.RWMutex
	subscribers  []regexSubscription
}

func NewMemoryBus() *MemoryBus {
	return &MemoryBus{
		subscribers: make([]regexSubscription, 0),
	}
}

func (b *MemoryBus) Publish(
	ctx context.Context, 
	event domain.Event,
) error {
	b.mu.RLock()
	eventType := event.GetEventType()

	var handlers []EventHandler
	for _, sub := range b.subscribers {
		if sub.pattern.MatchString(eventType) {
			handlers = append(handlers, sub.handler)
		}
	}
	b.mu.RUnlock()

	for _, handler := range handlers {
		if err := handler(ctx, event); err != nil {
			return fmt.Errorf("handler failed for event %s: %w", eventType, err)
		}
	}

	return nil
}

func (b *MemoryBus) Subscribe(
	eventTypeOrPattern string, 
	handler EventHandler,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	compiled, err := regexp.Compile(eventTypeOrPattern)
	if err != nil {
		return fmt.Errorf("invalid pattern %s: %w", eventTypeOrPattern, err)
	}

	b.subscribers = append(b.subscribers, regexSubscription{
		pattern: compiled,
		handler: handler,
		raw:     eventTypeOrPattern,
	})
	
	return nil
}
