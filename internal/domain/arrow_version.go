package domain

import (
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
)

// ArrowVersion is the compiled, runtime-ready representation of a single installed
// arrow version. Unlike ArrowManifest (the vault/manifold transfer type), this type
// stores resolved DependencyEdges and tracks install metadata used by the update flow.
type ArrowVersion struct {
	ArrowMeta
	Variables     []Variable          `json:"variables"`
	Netbridge     []netbridge.PortDef `json:"netbridge"`
	Targets       map[OS]Target       `json:"targets"`
	InstalledAt   time.Time           `json:"installed_at"`
	DirectInstall bool                `json:"direct_install"`
	InstalledRef  string              `json:"installed_ref"`
}
