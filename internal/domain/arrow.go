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
	ArrowMeta                               // Name, Description, License, etc.
	Variables           []Variable          `yaml:"variables"  json:"variables"`
	Netbridge           []netbridge.PortDef `yaml:"netbridge"  json:"netbridge"`
	Targets             map[OS]Target       `json:"targets"`
	InstalledAt         time.Time           `json:"installed_at"`
	UserInstalled       bool                `json:"user_installed"`
	InstalledRef        string              `json:"installed_ref"`
	InstalledConstraint string              `json:"installed_constraint"`
	// UpgradedFromNs is set only on the arrow.upgraded.* event; it names the
	// old namespace that was replaced so the runtime reaction can clean up.
	UpgradedFromNs Namespace `json:"upgraded_from_ns,omitempty"`
}

// ArrowMeta carries gorm tags so read models can embed it instead of restating
// its columns. Maintainers, Credits and Tags are ignored on purpose: they are
// slices, which cannot be columns, so a read model that needs them normalises
// them into a table of its own.
//
// There is no version here. An arrow's version is the ref its namespace names,
// and that ref is already the key every read model, cache entry and aggregate
// is filed under; a copy of it on the manifest could only ever disagree.
type ArrowMeta struct {
	Name        string     `yaml:"name"        json:"name"        gorm:"column:name"`
	Description string     `yaml:"description" json:"description" gorm:"column:description"`
	License     string     `yaml:"license"     json:"license"     gorm:"column:license"`
	URL         string     `yaml:"url"         json:"url"         gorm:"column:url"`
	Maintainers []Credit   `yaml:"maintainers" json:"maintainers" gorm:"-"`
	Credits     []Credit   `yaml:"credits"     json:"credits"     gorm:"-"`
	Tags        []string   `yaml:"tags"        json:"tags"        gorm:"-"`
	Media       ArrowMedia `yaml:"media"       json:"media"       gorm:"embedded"`
}

type ArrowMedia struct {
	Icon   string `yaml:"icon"   json:"icon"   gorm:"column:icon"`
	Banner string `yaml:"banner" json:"banner" gorm:"column:banner"`
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
