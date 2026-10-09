package manifold

import (
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher"
)

type Option func(*manifold)

// WithRefsDir keeps each repository's refs in dir, so a restart starts with
// them instead of asking every host again.
func WithRefsDir(
	dir string,
) Option {
	return func(m *manifold) {
		m.snapshots.Persist(dir)
	}
}

func WithFletcher(
	enabled bool,
) Option {
	return func(m *manifold) {
		m.fl = nil
		if enabled {
			m.fl = fletcher.New(m.hosts, fletcher.Releases{
				LatestStable:  m.ResolveLatestStable,
				Channels:      m.ListChannels,
				DefaultBranch: m.ResolveDefaultBranch,
			}, m.timeout)
		}
	}
}
