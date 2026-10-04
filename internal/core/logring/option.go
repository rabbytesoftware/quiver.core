package logring

import "time"

// Option configures New.
type Option func(*options)

// WithClock replaces the clock the ring measures subscriber stalls with. The
// default is time.Now; a test passes a controllable one, which also keeps it
// independent of the host's clock resolution (a Windows tick is coarser than
// the gap between two statements).
func WithClock(
	now func() time.Time,
) Option {
	return func(o *options) { o.now = now }
}
