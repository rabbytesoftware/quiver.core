package core

import (
	"github.com/rabbytesoftware/quiver.core/internal/core/logger"
	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

type options struct {
	ring logring.Ring
}

// Option configures New and NewAt.
type Option func(*options)

// WithLogRing makes the process logger also capture its records in ring.
func WithLogRing(
	ring logring.Ring,
) Option {
	return func(o *options) { o.ring = ring }
}

func (o options) loggerOptions() []logger.Option {
	if o.ring == nil {
		return nil
	}
	return []logger.Option{logger.WithRing(o.ring)}
}
