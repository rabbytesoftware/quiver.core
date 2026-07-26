package domain

import (
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain/netbridge"
)

const (
	MaxNameLength        = 255
	MaxDescriptionLength = 1000
	VersionLatestRef     = "latest"
	MethodInstall        = "_install"
	MethodUninstall      = "_uninstall"
	MethodUpdate         = "_update"
	MethodExecute        = "_execute"
	MethodStop           = "_stop"
)

// Arrow is the single canonical aggregate for an installed namespace@ref.
// When used as a parsed manifest (vault/manifold contexts) the installation
// fields (InstalledAt, InstalledRef, InstalledConstraint, UserInstalled) are zero.
type Arrow struct {
	Namespace           Namespace           `json:"namespace"`
	ArrowMeta                               // Name, Description, Version, etc.
	Variables           []Variable          `yaml:"variables"  json:"variables"`
	Netbridge           []netbridge.PortDef `yaml:"netbridge"  json:"netbridge"`
	Targets             map[OS]Target       `json:"targets"`
	InstalledAt         time.Time           `json:"installed_at"`
	UserInstalled       bool                `json:"user_installed"`
	InstalledRef        string              `json:"installed_ref"`
	InstalledConstraint string              `json:"installed_constraint"`
	// ResolvedBranch is where the manifest physically came from, which differs
	// from InstalledRef whenever the namespace carried no ref: the ref is what
	// was asked for, the branch is what the default-branch list settled on.
	// Empty means unknown, which resolves through the list as before.
	ResolvedBranch string `json:"resolved_branch"`
	// UpgradedFromNs is set only on the arrow.upgraded.* event; it names the
	// old namespace that was replaced so the runtime reaction can clean up.
	UpgradedFromNs Namespace `json:"upgraded_from_ns,omitempty"`
}

type ArrowMeta struct {
	Name        string     `yaml:"name"        json:"name"`
	Description string     `yaml:"description" json:"description"`
	Version     string     `yaml:"version"     json:"version"`
	License     string     `yaml:"license"     json:"license"`
	URL         string     `yaml:"url"         json:"url"`
	Maintainers []Credit   `yaml:"maintainers" json:"maintainers"`
	Credits     []Credit   `yaml:"credits"     json:"credits"`
	Tags        []string   `yaml:"tags"        json:"tags"`
	Media       ArrowMedia `yaml:"media"       json:"media"`
}

type ArrowMedia struct {
	Icon   string `yaml:"icon"   json:"icon"`
	Banner string `yaml:"banner" json:"banner"`
}

type ArrowState string

const (
	ArrowStateAbsent       ArrowState = "absent"
	ArrowStateInstalling   ArrowState = "installing"
	ArrowStateUpdating     ArrowState = "updating"
	ArrowStateReady        ArrowState = "ready"
	ArrowStateRunning      ArrowState = "running"
	ArrowStateStopping     ArrowState = "stopping"
	ArrowStateDraining     ArrowState = "draining"
	ArrowStateDetached     ArrowState = "detached"
	ArrowStateUninstalling ArrowState = "uninstalling"
	ArrowStateRemoved      ArrowState = "removed"
	ArrowStateOutdated     ArrowState = "outdated"
)

var transitions = map[ArrowState][]ArrowState{
	ArrowStateAbsent:       {ArrowStateReady},
	ArrowStateReady:        {ArrowStateRunning, ArrowStateInstalling, ArrowStateUninstalling, ArrowStateUpdating, ArrowStateOutdated},
	ArrowStateRunning:      {ArrowStateStopping, ArrowStateDetached},
	ArrowStateStopping:     {ArrowStateReady, ArrowStateDraining},
	ArrowStateDraining:     {ArrowStateReady},
	ArrowStateDetached:     {ArrowStateReady, ArrowStateStopping},
	ArrowStateInstalling:   {ArrowStateReady, ArrowStateAbsent},
	ArrowStateUninstalling: {ArrowStateAbsent, ArrowStateReady},
	ArrowStateUpdating:     {ArrowStateReady, ArrowStateAbsent},
	ArrowStateRemoved:      {},
	ArrowStateOutdated:     {ArrowStateReady, ArrowStateUninstalling},
}

func (s ArrowState) CanTransitionTo(
	target ArrowState,
) bool {
	for _, allowed := range transitions[s] {
		if allowed == target {
			return true
		}
	}
	return false
}

func (s ArrowState) IsActive() bool {
	return s == ArrowStateRunning ||
		s == ArrowStateStopping ||
		s == ArrowStateDraining ||
		s == ArrowStateInstalling ||
		s == ArrowStateUpdating
}
