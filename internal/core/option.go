package core

import "github.com/rabbytesoftware/quiver.core/internal/core/logring"

// Option configures New and NewAt.
type Option func(*options)

// WithLogRing makes the process logger also capture its records in ring.
func WithLogRing(
	ring logring.Ring,
) Option {
	return func(o *options) { o.ring = ring }
}
