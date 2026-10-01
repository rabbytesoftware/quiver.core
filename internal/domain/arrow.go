package domain

import (
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain/netbridge"
)

const (
	MaxNameLength        = 255
	MaxDescriptionLength = 1000
	MaxReadmeLength      = 256000
	MethodInstall        = "_install"
	MethodUninstall      = "_uninstall"
	MethodUpdate         = "_update"
	MethodExecute        = "_execute"
	MethodStop           = "_stop"
)

// Arrow is the single canonical aggregate for an installed namespace@selector.
// When used as a parsed manifest (vault/manifold contexts) the installation
// fields (InstalledAt, UserInstalled, SelectorKind, Resolved, Available) are
// zero.
type Arrow struct {
	Namespace Namespace           `json:"namespace"`
	ArrowMeta                     // Name, Description, License, etc.
	Variables []Variable          `yaml:"variables"  json:"variables"`
	Netbridge []netbridge.PortDef `yaml:"netbridge"  json:"netbridge"`
	Targets   map[OS]Target       `json:"targets"`
	// Readme is the prose surrounding the fenced manifest block when the arrow
	// is delivered as ARROW.md; it is empty for the plain arrow.yaml form. It
	// lives here rather than on ArrowMeta so it never rides into the lightweight
	// catalog/search row, which embeds ArrowMeta directly.
	Readme        string    `yaml:"readme,omitempty" json:"readme,omitempty"`
	InstalledAt   time.Time `json:"installed_at"`
	UserInstalled bool      `json:"user_installed"`
	// LastUsedAt is stamped when an _execute run completes successfully; zero
	// means the arrow has never been run.
	LastUsedAt time.Time `json:"last_used_at"`
	// SelectorKind names the type of selector this row tracks.
	SelectorKind SelectorKind `json:"selector_kind,omitempty" yaml:"selector_kind,omitempty"`
	// Resolved carries the installed ref and its resolved commit.
	Resolved Resolved `json:"resolved" yaml:"resolved"`
	// Available carries a newly available ref and its commit, or nil when no newer version is available.
	Available *Available `json:"available,omitempty" yaml:"available,omitempty"`
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
	Name        string          `yaml:"name"        json:"name"        gorm:"column:name"`
	Description string          `yaml:"description" json:"description" gorm:"column:description"`
	License     string          `yaml:"license"     json:"license"     gorm:"column:license"`
	URL         string          `yaml:"url"         json:"url"         gorm:"column:url"`
	Maintainers []Credit        `yaml:"maintainers" json:"maintainers" gorm:"-"`
	Credits     []Credit        `yaml:"credits"     json:"credits"     gorm:"-"`
	Tags        []string        `yaml:"tags"        json:"tags"        gorm:"-"`
	Media       ArrowMedia      `yaml:"media"       json:"media"       gorm:"embedded"`
	Generator   *ArrowGenerator `yaml:"generator,omitempty" json:"generator,omitempty" gorm:"-"`
}

func (a Arrow) Origin() string {
	if a.Generator == nil || a.Generator.Name == "" {
		return ArrowOriginDeclared
	}
	return ArrowOriginInferred
}

func (a Arrow) Confidence() string {
	if a.Origin() == ArrowOriginDeclared {
		return ""
	}
	return a.Generator.Confidence
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
	ArrowStateDetached:     {ArrowStateReady, ArrowStateStopping, ArrowStateRunning},
	ArrowStateInstalling:   {ArrowStateReady, ArrowStateAbsent},
	ArrowStateUninstalling: {ArrowStateAbsent, ArrowStateReady},
	ArrowStateUpdating:     {ArrowStateReady, ArrowStateAbsent},
	ArrowStateRemoved:      {},
	// Outdated reaches Running because it is not a busy state: an arrow a
	// version check found a newer release for is still installed and still
	// idle, and running it is the product's central action. Applying the
	// update it now has available stays an explicit trigger, never a gate
	// that has to be passed before the arrow can be used again.
	ArrowStateOutdated: {ArrowStateReady, ArrowStateRunning, ArrowStateUninstalling},
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
