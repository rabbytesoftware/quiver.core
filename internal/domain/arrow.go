package domain

import (
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
)

const (
	MaxNameLength        = 255
	MaxDescriptionLength = 1000
	VersionLatestRef     = "latest"
	MethodInstall        = "_install"
	MethodUninstall      = "_uninstall"
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
}

// ArrowManifest is a type alias for Arrow kept for source compatibility.
// Callers in the translator/ruleset/compiler chain use *ArrowManifest and
// compile unchanged. New code should use *Arrow directly.
type ArrowManifest = Arrow

type ArrowMeta struct {
	Name        string   `yaml:"name"        json:"name"`
	Description string   `yaml:"description" json:"description"`
	Version     string   `yaml:"version"     json:"version"`
	License     string   `yaml:"license"     json:"license"`
	URL         string   `yaml:"url"         json:"url"`
	Maintainers []Credit `yaml:"maintainers" json:"maintainers"`
	Credits     []Credit `yaml:"credits"     json:"credits"`
	Tags        []string `yaml:"tags"        json:"tags"`
}

type ArrowState string

const (
	ArrowStateAbsent       ArrowState = "absent"
	ArrowStateInstalling   ArrowState = "installing"
	ArrowStateExecuting    ArrowState = "executing"
	ArrowStateUpdating     ArrowState = "updating"
	ArrowStateReady        ArrowState = "ready"
	ArrowStateRunning      ArrowState = "running"
	ArrowStateStopping     ArrowState = "stopping"
	ArrowStateUninstalling ArrowState = "uninstalling"
	ArrowStateRemoved      ArrowState = "removed"
)

// IsActive returns true when the arrow is in any transitional or running state.
func (s ArrowState) IsActive() bool {
	return s == ArrowStateRunning ||
		s == ArrowStateInstalling ||
		s == ArrowStateStopping ||
		s == ArrowStateExecuting
}
