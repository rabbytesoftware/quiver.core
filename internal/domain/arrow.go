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
	Namespace Namespace
	Manifest  ArrowManifest
	Removed   bool
}

type ArrowManifest struct {
	Name         string
	Description  string
	Version      string
	License      string
	URL          string
	Maintainers  []string
	Credits      []string
	Tags         []string
	Requirements Requirement
	Dependencies []Namespace
	Variables    []Variable
	Netbridge    []netbridge.PortDef
	Lifecycle    Lifecycle
	Methods      map[string]Method
}

type Lifecycle struct {
	Install   []step.Step
	Execute   []step.Step
	Stop      []step.Step
	Uninstall []step.Step
}

type Method struct {
	AvailableIn []ArrowState
	Steps       []step.Step
}
