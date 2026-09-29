package manifold

import (
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

type (
	// Host answers the host-specific questions manifest resolution runs into:
	// where a raw file lives and which refs a repository defaults to.
	Host = hosts.Host

	// HostLookup resolves the host serving a namespace, reporting false when
	// none does.
	HostLookup = hosts.Lookup
)
