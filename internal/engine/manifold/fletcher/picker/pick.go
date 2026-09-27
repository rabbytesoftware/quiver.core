package picker

import (
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

type Pick struct {
	Asset     hosts.Asset
	Format    Format
	Match     Match
	NameMatch bool
	GUI       bool
}
