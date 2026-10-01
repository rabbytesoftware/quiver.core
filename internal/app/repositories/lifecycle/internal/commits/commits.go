package commits

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// Commits tracks the update commits running detached, so a shutdown can wait
// for them before the stores they write to close.
type Commits interface {
	// Begin registers a commit of ns and reports whether it may run: once a
	// drain began, none may.
	Begin(ns domain.Namespace) bool
	Done(ns domain.Namespace)
	InFlight(ns domain.Namespace) bool
	IsDraining() bool
	// Bound derives a commit's context: it ends after timeout, or as soon as
	// a drain gives up waiting for it.
	Bound(
		parent context.Context,
		timeout time.Duration,
	) (context.Context, context.CancelFunc)
	// Drain waits for every commit in flight and refuses new ones. When ctx
	// ends first it aborts them and reports why: those rows stay outdated and
	// their next update runs again.
	Drain(ctx context.Context) error
}

type commits struct {
	mu       sync.Mutex
	wg       sync.WaitGroup
	draining bool
	running  map[domain.Namespace]int
	abort    chan struct{}
	aborted  bool
}

func New() Commits {
	return &commits{
		running: make(map[domain.Namespace]int),
		abort:   make(chan struct{}),
	}
}

// Begin adds under the same lock that flips draining, so no Add can race the
// drain's Wait.
func (c *commits) Begin(
	ns domain.Namespace,
) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.draining {
		return false
	}
	c.wg.Add(1)
	c.running[ns]++
	return true
}

func (c *commits) Done(
	ns domain.Namespace,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.running[ns]--
	if c.running[ns] <= 0 {
		delete(c.running, ns)
	}
	c.wg.Done()
}

func (c *commits) InFlight(
	ns domain.Namespace,
) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running[ns] > 0
}

func (c *commits) IsDraining() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.draining
}

func (c *commits) Bound(
	parent context.Context,
	timeout time.Duration,
) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	go func() {
		select {
		case <-c.abort:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

func (c *commits) Drain(
	ctx context.Context,
) error {
	if err := c.drain(ctx); err != nil {
		slog.WarnContext(ctx, "update: shutdown abandoned commits in flight; their rows stay outdated", "err", err)
		return fmt.Errorf("drain update commits: %w", err)
	}
	return nil
}

// drain aborts the running commits when ctx ends first, so none outlives the
// stores it writes to by more than its own unwinding.
func (c *commits) drain(
	ctx context.Context,
) error {
	c.mu.Lock()
	c.draining = true
	idle := len(c.running) == 0
	c.mu.Unlock()
	if idle {
		return nil
	}

	settled := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(settled)
	}()

	select {
	case <-settled:
		return nil
	case <-ctx.Done():
		c.abortRunning()
		return ctx.Err()
	}
}

func (c *commits) abortRunning() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.aborted {
		return
	}
	c.aborted = true
	close(c.abort)
}
