package shutdown

import (
	"context"
	"time"
)

// Phase is one step of a shutdown sequence: a name for the error chain, the
// deadline the step may spend, and the work itself. A zero Timeout leaves the
// step unbounded. Split computes the share itself and ignores Timeout.
type Phase struct {
	Name    string
	Timeout time.Duration
	Run     func(ctx context.Context) error
}
