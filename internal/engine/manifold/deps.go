package manifold

import (
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

type (
	// Host answers the host-specific questions manifest resolution runs into:
	// where a raw file lives, which refs a repository defaults to, and which ref
	// its latest release carries.
	Host = hosts.Host

	// HostLookup resolves the host serving a namespace, reporting false when
	// none does.
	HostLookup = hosts.Lookup
)
