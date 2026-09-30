package recommendationinternal

import (
	"context"
	"fmt"
	"sync"
)

// Hooks holds the callbacks fired after a refresh replaced at least one shelf.
type Hooks struct {
	mu        sync.Mutex
	refreshed []func(ctx context.Context)
}

// OnRefreshed registers fn. Callbacks run in registration order on the refresh
// goroutine, so they must not block.
func (h *Hooks) OnRefreshed(
	fn func(ctx context.Context),
) error {
	if fn == nil {
		return fmt.Errorf("recommendation: refreshed callback must not be nil")
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.refreshed = append(h.refreshed, fn)
	return nil
}

// Fire runs every registered callback.
func (h *Hooks) Fire(
	ctx context.Context,
) {
	h.mu.Lock()
	fns := append([]func(ctx context.Context){}, h.refreshed...)
	h.mu.Unlock()

	for _, fn := range fns {
		fn(ctx)
	}
}
