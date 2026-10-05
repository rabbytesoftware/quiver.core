package logger

import "github.com/rabbytesoftware/quiver.core/internal/core/logring"

// Option configures Init and InitAt.
type Option func(*options)

// WithRing makes the process logger also store every record in ring, whatever
// the destination it writes to, so the console can replay and follow them. A
// nil ring leaves the logger as it is.
func WithRing(
	ring logring.Ring,
) Option {
	return func(
		o *options,
	) {
		o.ring = ring
	}
}
