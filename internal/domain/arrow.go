package domain

import (
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
)

const (
	MaxNameLength        = 255
	MaxDescriptionLength = 1000
)

type Arrow struct {
	Namespace Namespace `json:"namespace"`
	Manifest  ArrowManifest `json:"manifest"`
	Removed   bool          `json:"removed"`
}

type ArrowManifest struct {
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	Version      string              `json:"version"`
	License      string              `json:"license"`
	URL          string              `json:"url"`
	Maintainers  []string            `json:"maintainers"`
	Credits      []Credit            `json:"credits"`
	Tags         []string            `json:"tags"`
	Requirements Requirement         `json:"requirements"`
	Dependencies []Namespace         `json:"dependencies"`
	Variables    []Variable          `json:"variables"`
	Netbridge    []netbridge.PortDef `json:"netbridge"`
	Lifecycle    Lifecycle           `json:"lifecycle"`
	Methods      map[string]Method   `json:"methods"`
}

type Lifecycle struct {
	Install   []step.Step `json:"install"`
	Execute   []step.Step `json:"execute"`
	Stop      []step.Step `json:"stop"`
	Uninstall []step.Step `json:"uninstall"`
}

type Method struct {
	AvailableIn []ArrowState `json:"available_in"`
	Steps       []step.Step  `json:"steps"`
}
