package manifold

import (
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher"
)

type Option func(*manifold)

func WithFletcher(
	enabled bool,
) Option {
	return func(m *manifold) {
		m.fl = nil
		if enabled {
			m.fl = fletcher.New(m.hosts, m, m.timeout)
		}
	}
}
