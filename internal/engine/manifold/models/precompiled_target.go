package models

import (
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
)

// PrecompiledTarget is the raw, Overrideable-bearing shape parsed from a manifest.
// It is used internally by the manifold engine before OS-specific compilation.
// Exports uses step.Overrideable[string] to allow values to vary per arch within a glob target.
type PrecompiledTarget struct {
	Base         string                               `yaml:"base"         json:"base"`
	Requirements domain.Requirement                   `yaml:"requirements" json:"requirements"`
	Tools        []domain.Namespace                   `yaml:"tools"        json:"tools"`
	Services     []domain.Namespace                   `yaml:"services"     json:"services"`
	Exports      map[string]step.Overrideable[string] `yaml:"exports"      json:"exports"`
	Lifecycle    domain.TargetLifecycle               `yaml:"lifecycle"    json:"lifecycle"`
	Methods      map[string]domain.Method             `yaml:"methods"      json:"methods"`
}
