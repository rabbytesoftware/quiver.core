package core

import "github.com/rabbytesoftware/quiver.core/internal/core/logring"

// Option configures New and NewAt.
type Option func(*options)

// WithLogRing makes the process logger also store every record in ring, which
// is what the console's log stream serves. A nil ring leaves the logger as it
// is.
func WithLogRing(
	ring logring.Ring,
) Option {
	return func(
		o *options,
	) {
		o.ring = ring
	}
}
