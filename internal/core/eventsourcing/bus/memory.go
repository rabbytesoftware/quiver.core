package bus

import (
	"context"
	"fmt"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

type MemoryBus struct {
	mu         sync.RWMutex
	subscribers map[string][]EventHandler
}

func NewMemoryBus() *MemoryBus {
	return &MemoryBus{
		subscribers: make(map[string][]EventHandler),
	}
}

func (b *MemoryBus) Publish(ctx context.Context, event domain.Event) error {
	b.mu.RLock()
	handlers := b.subscribers[event.GetEventType()]
	b.mu.RUnlock()

	for _, handler := range handlers {
		if err := handler(ctx, event); err != nil {
			return fmt.Errorf("handler failed for event %s: %w", event.GetEventType(), err)
		}
	}

	return nil
}

func (b *MemoryBus) Subscribe(eventType string, handler EventHandler) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.subscribers[eventType] = append(b.subscribers[eventType], handler)
	return nil
}

