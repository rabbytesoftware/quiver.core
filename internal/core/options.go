package core

import (
	"github.com/rabbytesoftware/quiver.core/internal/core/logger"
	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

type options struct {
	ring logring.Ring
}

func (o options) loggerOptions() []logger.Option {
	if o.ring == nil {
		return nil
	}
	return []logger.Option{logger.WithRing(o.ring)}
}
