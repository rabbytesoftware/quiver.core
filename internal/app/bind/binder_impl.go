package bind

import (
	"context"
	"errors"
	"fmt"
	"sync"

	asynxModels "github.com/char2cs/asynx/models"
)

type binder[T any] struct {
	mu      sync.RWMutex
	durable []durableRoute[T]
	sync    []syncRoute[T]
	ready   chan struct{}
	once    sync.Once
}

// New returns a Binder that is not ready: its durable dispatch blocks until
// Ready is called.
func New[T any]() Binder[T] {
	return &binder[T]{ready: make(chan struct{})}
}

func (b *binder[T]) Durable(
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

func (b *binder[T]) Sync(
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

func (b *binder[T]) Ready() {
	b.once.Do(b.release)
}

func (b *binder[T]) DispatchDurable(
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

func (b *binder[T]) DispatchSync(
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

func (b *binder[T]) release() {
	close(b.ready)
}

func (b *binder[T]) waitReady(
	ctx context.Context,
) error {
	select {
	case <-b.ready:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("bind: waiting for wiring: %w", ctx.Err())
	}
}

func (b *binder[T]) durableRoutes() []durableRoute[T] {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return append([]durableRoute[T](nil), b.durable...)
}

func (b *binder[T]) syncRoutes() []syncRoute[T] {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return append([]syncRoute[T](nil), b.sync...)
}
