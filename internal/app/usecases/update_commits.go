package usecases

import (
	"context"
	"sync"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// updateCommits tracks the update commits running detached, so a shutdown
// can wait for them before the stores they write to close.
type updateCommits struct {
	mu       sync.Mutex
	wg       sync.WaitGroup
	draining bool
	running  map[domain.Namespace]int
	abort    chan struct{}
	aborted  bool
}

func newUpdateCommits() *updateCommits {
	return &updateCommits{
		running: make(map[domain.Namespace]int),
		abort:   make(chan struct{}),
	}
}

// begin registers a commit of ns and reports whether it may run: once a
// drain began, none may. The Add happens under the same lock that flips
// draining, so no Add can race the drain's Wait.
func (c *updateCommits) begin(
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

func (c *updateCommits) done(
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

func (c *updateCommits) inFlight(
	ns domain.Namespace,
) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running[ns] > 0
}

func (c *updateCommits) isDraining() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.draining
}

// bound derives a commit's context: it ends after timeout, or as soon as a
// drain gives up waiting for it.
func (c *updateCommits) bound(
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

// drain refuses every later commit and waits for the running ones. When ctx
// ends first it aborts them, so none outlives the stores it writes to by
// more than its own unwinding.
func (c *updateCommits) drain(
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

func (c *updateCommits) abortRunning() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.aborted {
		return
	}
	c.aborted = true
	close(c.abort)
}
