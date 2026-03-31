package step

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// OverrideableString is a string value that can be overridden per OS/arch.
// It supports a default value and a map of OS/arch-specific overrides.
//
// YAML usage:
//
//	command: "./install.sh"  # scalar: default only
//	url:
//	  default: "https://example.com/binary.tar.gz"
//	  linux/amd64: "https://example.com/linux-amd64.tar.gz"
//	  darwin/arm64: "https://example.com/darwin-arm64.tar.gz"
type OverrideableString struct {
	Default string
	OSArch  map[string]string
}

// Resolve returns the OS/arch-specific value if present, otherwise the Default.
func (o OverrideableString) Resolve(osArch string) string {
	if v, ok := o.OSArch[osArch]; ok {
		return v
	}
	return o.Default
}

// UnmarshalYAML handles both scalar and map forms.
// If the YAML value is a scalar (string), it becomes the Default.
// If the YAML value is a map, keys are OS/arch identifiers or "default".
func (o *OverrideableString) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		o.Default = value.Value
		return nil
	}

	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("expected string or map, got %v", value.Kind)
	}

	var m map[string]string
	if err := value.Decode(&m); err != nil {
		return fmt.Errorf("failed to decode override map: %w", err)
	}

	o.Default = m["default"]
	delete(m, "default")

	if len(m) > 0 {
		o.OSArch = m
	}

	return nil
}
