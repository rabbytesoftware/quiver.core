package versioning

import (
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/versioning/internal/admit"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/versioning/internal/selector"
)

var (
	// ErrUnknownSelector reports a selector or ref a snapshot does not hold.
	ErrUnknownSelector = selector.ErrUnknownSelector

	// ErrNotAdmitted reports a ref its selector could never resolve to.
	ErrNotAdmitted = admit.ErrNotAdmitted
)
