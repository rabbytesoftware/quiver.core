package logger

import "github.com/rabbytesoftware/quiver.core/internal/core/logring"

type options struct {
	ring logring.Ring
}

// Option configures Init and InitAt.
type Option func(*options)

// WithRing makes the logger also capture every record in ring, as structured
// data, regardless of where the records are written.
func WithRing(
	ring logring.Ring,
) Option {
	return func(o *options) { o.ring = ring }
}
