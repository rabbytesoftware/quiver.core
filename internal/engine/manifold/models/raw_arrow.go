package models

import (
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
)

// RawArrow is the top-level YAML structure for an arrow@v0 manifest.
// It nests metadata in a block that domain.ArrowManifest does not, and
// uses RawStep-based types for lifecycle and methods before assembly.
type RawArrow struct {
	Schema       string                `yaml:"schema"`
	Metadata     RawMetadata           `yaml:"metadata"`
	Requirements RawRequirements       `yaml:"requirements"`
	Dependencies []string              `yaml:"dependencies"`
	Variables    []domain.Variable     `yaml:"variables"`
	Netbridge    []netbridge.PortDef   `yaml:"netbridge"`
	Lifecycle    RawLifecycle          `yaml:"lifecycle"`
	Methods      map[string]RawMethod  `yaml:"methods"`
}
