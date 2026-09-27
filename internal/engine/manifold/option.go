package manifold

import (
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher"
)

type Option func(*manifold)

func WithFletcher(
	fl fletcher.Fletcher,
) Option {
	return func(m *manifold) {
		m.fl = fl
	}
}
