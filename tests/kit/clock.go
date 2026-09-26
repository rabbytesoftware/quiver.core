//go:build integration

package kit

import (
	"sync"
	"time"
)

// AdvanceableClock is a thread-safe, manually-advanceable clock for a test
// that needs to move time forward deterministically -- past a cache or
// throttle TTL, say -- without a real sleep. Safe for concurrent use: the
// daemon's own goroutines call Now() while the test's own goroutine calls
// Advance().
type AdvanceableClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewAdvanceableClock returns a clock starting at start.
func NewAdvanceableClock(start time.Time) *AdvanceableClock {
	return &AdvanceableClock{now: start}
}

// Now reports the clock's current time. Pass this method value directly
// wherever a func() time.Time is expected (e.g. kit.WithClock(clock.Now)).
func (c *AdvanceableClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward by d.
func (c *AdvanceableClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
