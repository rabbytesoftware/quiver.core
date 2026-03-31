package step

import (
	"encoding/json"
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

// fromMap populates the OverrideableString from a flat string map.
// The "default" key becomes Default; all other keys are OS/arch overrides.
func (o *OverrideableString) fromMap(m map[string]string) {
	o.Default = m["default"]
	delete(m, "default")
	if len(m) > 0 {
		o.OSArch = m
	}
}

// toMap converts the OverrideableString back into a flat string map,
// with "default" as a key alongside OS/arch overrides.
func (o OverrideableString) toMap() map[string]string {
	m := make(map[string]string, len(o.OSArch)+1)
	m["default"] = o.Default
	for k, v := range o.OSArch {
		m[k] = v
	}
	return m
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

	o.fromMap(m)
	return nil
}

// MarshalYAML emits a scalar string when there are no OS/arch overrides,
// or a map with "default" and OS/arch keys when overrides exist.
func (o OverrideableString) MarshalYAML() (interface{}, error) {
	if len(o.OSArch) == 0 {
		return o.Default, nil
	}
	return o.toMap(), nil
}

// UnmarshalJSON handles both scalar string and object (map) forms.
// If JSON value is a string, it becomes the Default.
// If JSON value is an object, keys are OS/arch identifiers or "default".
func (o *OverrideableString) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		o.Default = s
		return nil
	}

	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("expected string or object, got invalid JSON: %w", err)
	}

	o.fromMap(m)
	return nil
}

// MarshalJSON emits a scalar string when there are no OS/arch overrides,
// or an object with "default" and OS/arch keys when overrides exist.
func (o OverrideableString) MarshalJSON() ([]byte, error) {
	if len(o.OSArch) == 0 {
		return json.Marshal(o.Default)
	}
	return json.Marshal(o.toMap())
}
