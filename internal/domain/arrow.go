package domain

import (
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain/netbridge"
)

const (
	MaxNameLength        = 255
	MaxDescriptionLength = 1000
	MaxReadmeLength      = 256000
	VersionLatestRef     = "latest"
	MethodInstall        = "_install"
	MethodUninstall      = "_uninstall"
	MethodUpdate         = "_update"
	MethodExecute        = "_execute"
	MethodStop           = "_stop"
)

// Arrow is the single canonical aggregate for an installed namespace@ref.
// When used as a parsed manifest (vault/manifold contexts) the installation
// fields (InstalledAt, InstalledConstraint, UserInstalled) are zero.
//
// There is no installed ref either. The aggregate is keyed by namespace@ref, so
// the only ref that can ever be installed under it is the one Namespace already
// names; InstalledAt alone says whether that install has happened.
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
	Readme              string    `yaml:"readme,omitempty" json:"readme,omitempty"`
	InstalledAt         time.Time `json:"installed_at"`
	UserInstalled       bool      `json:"user_installed"`
	InstalledConstraint string    `json:"installed_constraint"`
	// LastUsedAt is stamped when an _execute run completes successfully; zero
	// means the arrow has never been run.
	LastUsedAt time.Time `json:"last_used_at"`
	// UpgradedFromNs is set only on the arrow.upgraded.* event; it names the
	// old namespace that was replaced so the runtime reaction can clean up.
	UpgradedFromNs Namespace `json:"upgraded_from_ns,omitempty"`
	// AlreadyReady is set only on an arrow.upgraded.* event raised after this
	// arrow's own update lifecycle already finished successfully: the software
	// at the new ref is already fetched, placed and running, so the reaction
	// that lands the new row must seed it Ready directly rather than install
	// it again. Never set on an upgrade_ref-driven swap, where the new ref
	// genuinely has not been installed yet.
	AlreadyReady bool `json:"already_ready,omitempty"`
	// RefIsBranch marks a namespace resolved onto a moving branch rather than
	// pinned as written or matched to a tag — stamped only by the
	// refless-resolution fallback when no stable release exists.
	RefIsBranch bool `json:"ref_is_branch,omitempty"`
	// RefCommitSHA is the commit hash the branch ref pointed at when resolved.
	// Meaningful only when RefIsBranch is true.
	RefCommitSHA string `json:"ref_commit_sha,omitempty"`
	// Outdated is true once a version check found a better ref available.
	Outdated bool `json:"outdated"`
	// RecommendedRef names the tag a version check found to replace the
	// installed ref. Empty when Outdated is true for plain branch drift with
	// no better named ref to switch to.
	RecommendedRef string `json:"recommended_ref"`
	// Channel names the release channel this arrow's default drift check
	// tracks (e.g. "stable", "rc", "beta"). Empty is treated as "stable" —
	// the default before any channel was ever explicitly selected.
	Channel string `json:"channel,omitempty"`
	// PinnedRef, when set, is the exact ref within Channel this arrow tracks,
	// overriding that channel's own latest. Empty means "track the channel's
	// latest", the plain behavior before any specific version was pinned.
	PinnedRef string `json:"pinned_ref,omitempty"`
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
