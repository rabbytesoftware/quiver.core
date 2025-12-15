package bus

import (
	"context"
	"sync"

	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/contracts"
	"github.com/rabbytesoftware/quiver/internal/core/eventsourcing/domain"
)

type MemoryEventBus struct {
	handlers map[string][]contracts.Handler
	mu       sync.RWMutex
}

func NewEventBus() contracts.EventBus {
	return &MemoryEventBus{
		handlers: make(map[string][]contracts.Handler),
	}
}

func (b *MemoryEventBus) Subscribe(eventType string, handler contracts.Handler) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.handlers[eventType] = append(b.handlers[eventType], handler)
	return nil
}

func (b *MemoryEventBus) Publish(ctx context.Context, event domain.Event) error {
	b.mu.RLock()
	handlers := b.handlers[event.GetEventType()]
	allHandlers := append([]contracts.Handler{}, handlers...)

	wildcardHandlers := b.handlers["*"]
	allHandlers = append(allHandlers, wildcardHandlers...)
	b.mu.RUnlock()

	var wg sync.WaitGroup
	errChan := make(chan error, len(allHandlers))

	for _, handler := range allHandlers {
		wg.Add(1)
		go func(h contracts.Handler) {
			defer wg.Done()
			if err := h(ctx, event); err != nil {
				errChan <- err
			}
		}(handler)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		return err
	}

	return nil
}
