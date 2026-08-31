package cascade

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"gorm.io/gorm"

	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/cascade/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// ForgetRuntimeFn forgets the runtime aggregate for ns.
type ForgetRuntimeFn func(ctx context.Context, ns domain.Namespace) error

// Cascade durably records arrows whose runtime still needs forgetting after
// removal, and finishes forgetting them off the caller's goroutine.
//
// asynx's Forget uses SendWait, which blocks until a worker on that
// aggregate's own asynx instance picks up the command. Calling Runtime.Forget
// inline from the OnArrowRemoved handler — itself running inside axArrow's own
// SendWait-driven dispatch (see app.newAsynx) — risks a cross-instance
// deadlock: an axArrow worker blocked waiting on an axRuntime worker that is
// itself blocked waiting on an axArrow worker. Enqueue never calls
// ForgetRuntimeFn itself, so the synchronous hook that calls it can never
// block on axRuntime.
type Cascade interface {
	// Enqueue durably records ns for cascade cleanup and returns as soon as the
	// row is persisted, then kicks a background Drain. Safe to call from
	// inside a synchronous asynx dispatch handler.
	Enqueue(ctx context.Context, ns domain.Namespace) error

	// Drain runs ForgetRuntimeFn for every namespace still pending, clearing
	// each on success and leaving the rest for the next call. A failure on one
	// namespace does not stop the others. Call at boot to finish cascades a
	// prior crash interrupted.
	Drain(ctx context.Context) error

	// Shutdown closes the gate so no further background drain starts, then
	// waits for any already running, bounded by ctx.
	Shutdown(ctx context.Context) error
}

type cascadeService struct {
	store         store.Store
	forgetRuntime ForgetRuntimeFn

	mu     sync.Mutex
	wg     sync.WaitGroup
	closed bool
}

// New builds a Cascade backed by db — the same handle the arrow and graph read
// models use, so its lifecycle is already owned elsewhere.
func New(
	db *gorm.DB,
	forgetRuntime ForgetRuntimeFn,
) (Cascade, error) {
	st, err := store.New(db)
	if err != nil {
		return nil, fmt.Errorf("cascade: %w", err)
	}
	return &cascadeService{store: st, forgetRuntime: forgetRuntime}, nil
}

func (c *cascadeService) Enqueue(ctx context.Context, ns domain.Namespace) error {
	if err := c.store.Enqueue(ctx, ns.String()); err != nil {
		return fmt.Errorf("cascade: enqueue %s: %w", ns, err)
	}

	if done, ok := c.tryAddDrain(); ok {
		go func() {
			defer done()
			if err := c.Drain(context.WithoutCancel(ctx)); err != nil {
				slog.ErrorContext(ctx, "forget cascade: drain", "err", err)
			}
		}()
	}

	return nil
}

// tryAddDrain registers one drain goroutine with the WaitGroup, mirroring
// runtimeRepository.tryAddDrain (internal/app/repositories/runtime/runtime.go).
func (c *cascadeService) tryAddDrain() (func(), bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, false
	}
	c.wg.Add(1)
	return c.wg.Done, true
}

func (c *cascadeService) Drain(ctx context.Context) error {
	pending, err := c.store.Pending(ctx)
	if err != nil {
		return fmt.Errorf("cascade: list pending: %w", err)
	}

	var errs []error
	for _, raw := range pending {
		if err := c.run(ctx, domain.Namespace(raw)); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (c *cascadeService) run(ctx context.Context, ns domain.Namespace) error {
	if err := c.forgetRuntime(ctx, ns); err != nil {
		return fmt.Errorf("cascade: forget runtime %s: %w", ns, err)
	}
	if err := c.store.Complete(ctx, ns.String()); err != nil {
		return fmt.Errorf("cascade: complete %s: %w", ns, err)
	}
	return nil
}

func (c *cascadeService) Shutdown(ctx context.Context) error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()

	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
