package fletcher

import (
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/fallback"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/gather"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/picker"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

const defaultFetchTimeout = 30 * time.Second

type (
	Fletcher = models.Fletcher
	Releases = models.Releases
)

func New(
	lookup hosts.Lookup,
	releases Releases,
	timeout time.Duration,
) Fletcher {
	if timeout == 0 {
		timeout = defaultFetchTimeout
	}
	lookup = hosts.Or(lookup)
	return fallback.New(lookup, releases, gather.New(lookup, picker.New(), timeout))
}
