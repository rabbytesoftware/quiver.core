package binder

import (
	"context"
	"errors"
	"fmt"
	"sync"

	asynxModels "github.com/char2cs/asynx/models"
)

// Binder routes the events of one asynx instance to handlers registered after
// the instance was built. Its dispatch methods are the catch-all durable
// subscriber and sync projection the instance is built with.
type Binder[T any] struct {
	mu      sync.RWMutex
	durable []durableRoute[T]
	sync    []syncRoute[T]
	ready   chan struct{}
	once    sync.Once
}

// New returns a Binder that is not ready: its durable dispatch blocks until
// Ready is called.
func New[T any]() *Binder[T] {
	return &Binder[T]{ready: make(chan struct{})}
}

// Durable adds a durable route. The pattern is what asynx's Subscribe takes:
// an exact event name, or a regular expression. It fails when the pattern is
// not a valid expression.
func (b *Binder[T]) Durable(
	pattern string,
	fn asynxModels.DurableHandler[T],
) error {
	match, err := matcher(pattern)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.durable = append(b.durable, durableRoute[T]{match: match, fn: fn})
	return nil
}

// Sync adds a sync-projection route. The pattern is what asynx's Subscribe
// takes. It fails when the pattern is not a valid expression.
func (b *Binder[T]) Sync(
	pattern string,
	fn SyncFn[T],
) error {
	match, err := matcher(pattern)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sync = append(b.sync, syncRoute[T]{match: match, fn: fn})
	return nil
}

// Ready releases the durable deliveries that were waiting for wiring. It is
// safe to call more than once.
func (b *Binder[T]) Ready() {
	b.once.Do(b.release)
}

// DispatchDurable is the catch-all durable subscriber. It waits for Ready,
// then runs every matching route; the errors are returned together after all
// ran, so one failing route is retried without starving its siblings of the
// attempt.
func (b *Binder[T]) DispatchDurable(
	ctx context.Context,
	evt asynxModels.Event[T],
) error {
	err := b.waitReady(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, route := range b.durableRoutes() {
		errs = append(errs, route.run(ctx, evt))
	}
	return errors.Join(errs...)
}

// DispatchSync is the catch-all sync projection. Routes run in registration
// order inside the event's transaction and the first error aborts it. It does
// not wait for Ready: no command reaches an aggregate before its repositories
// are wired, and blocking here would hold the transaction open.
func (b *Binder[T]) DispatchSync(
	ctx context.Context,
	tx asynxModels.Tx,
	evt asynxModels.Event[T],
) error {
	var err error
	routes := b.syncRoutes()
	for i := 0; i < len(routes) && err == nil; i++ {
		err = routes[i].run(ctx, tx, evt)
	}
	return err
}

func (b *Binder[T]) release() {
	close(b.ready)
}

func (b *Binder[T]) waitReady(
	ctx context.Context,
) error {
	select {
	case <-b.ready:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("binder: waiting for wiring: %w", ctx.Err())
	}
}

func (b *Binder[T]) durableRoutes() []durableRoute[T] {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return append([]durableRoute[T](nil), b.durable...)
}

func (b *Binder[T]) syncRoutes() []syncRoute[T] {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return append([]syncRoute[T](nil), b.sync...)
}
