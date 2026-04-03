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
	Namespace Namespace     `json:"namespace"`
	Manifest  ArrowManifest `json:"manifest"`
	Removed   bool          `json:"removed"`
}

type ArrowManifest struct {
	Name         string              `yaml:"name"         json:"name"`
	Description  string              `yaml:"description"  json:"description"`
	Version      string              `yaml:"version"      json:"version"`
	License      string              `yaml:"license"      json:"license"`
	URL          string              `yaml:"url"          json:"url"`
	Maintainers  []string            `yaml:"maintainers"  json:"maintainers"`
	Credits      []Credit            `yaml:"credits"      json:"credits"`
	Tags         []string            `yaml:"tags"         json:"tags"`
	Requirements Requirement         `yaml:"requirements" json:"requirements"`
	Dependencies []Namespace         `yaml:"dependencies" json:"dependencies"`
	Variables    []Variable          `yaml:"variables"    json:"variables"`
	Netbridge    []netbridge.PortDef `yaml:"netbridge"    json:"netbridge"`
	Lifecycle    Lifecycle           `yaml:"lifecycle"    json:"lifecycle"`
	Methods      map[string]Method   `yaml:"methods"      json:"methods"`
}

type Lifecycle struct {
	Install   step.StepList `yaml:"install"   json:"install"`
	Execute   step.StepList `yaml:"execute"   json:"execute"`
	Stop      step.StepList `yaml:"stop"      json:"stop"`
	Uninstall step.StepList `yaml:"uninstall" json:"uninstall"`
}

type Method struct {
	AvailableIn []ArrowState  `yaml:"available_in" json:"available_in"`
	Steps       step.StepList `yaml:"steps"        json:"steps"`
}
