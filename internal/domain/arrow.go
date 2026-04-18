package domain

import (
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
)

const (
	MaxNameLength        = 255
	MaxDescriptionLength = 1000
)

type Arrow struct {
	Namespace Namespace                `json:"namespace"`
	Versions  map[string]ArrowManifest `json:"versions"`
}

type ArrowManifest struct {
	ArrowMeta
	Variables     []Variable          `yaml:"variables" json:"variables"`
	Netbridge     []netbridge.PortDef `yaml:"netbridge" json:"netbridge"`
	Targets       map[OS]Target       `json:"targets"`
	InstalledAt   time.Time           `json:"installed_at"`
	UserInstalled bool                `json:"user_installed"`
}

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
	ArrowStateUpdating     ArrowState = "updating"
	ArrowStateReady        ArrowState = "ready"
	ArrowStateRunning      ArrowState = "running"
	ArrowStateStopping     ArrowState = "stopping"
	ArrowStateUninstalling ArrowState = "uninstalling"
	ArrowStateRemoved      ArrowState = "removed"
)
