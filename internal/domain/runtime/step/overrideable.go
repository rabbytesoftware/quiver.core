package step

import (
	"encoding/json"
	"fmt"
)

// Overrideable is a value that can be overridden per OS/arch.
// It supports a default value and a map of OS/arch-specific overrides.
//
// JSON usage:
//
//	"command": "echo hello"  # scalar: default only
//	"url": {
//	  "default": "https://example.com/binary.tar.gz",
//	  "linux/amd64": "https://example.com/linux-amd64.tar.gz",
//	  "darwin/arm64": "https://example.com/darwin-arm64.tar.gz"
//	}
type Overrideable[T any] struct {
	Default T
	OSArch  map[string]T
}

// Resolve returns the OS/arch-specific value if present, otherwise the Default.
func (o Overrideable[T]) Resolve(osArch string) T {
	if v, ok := o.OSArch[osArch]; ok {
		return v
	}
	return o.Default
}

// WithOverride adds or updates an OS/arch-specific override and returns the Overrideable for chaining.
func (o Overrideable[T]) WithOverride(osArch string, value T) Overrideable[T] {
	if o.OSArch == nil {
		o.OSArch = make(map[string]T)
	}
	o.OSArch[osArch] = value
	return o
}

// fromMap populates the Overrideable from a flat map.
// The "default" key becomes Default; all other keys are OS/arch overrides.
// Does not mutate the input map.
func (o *Overrideable[T]) fromMap(m map[string]T) {
	o.Default = m["default"]
	for k, v := range m {
		if k == "default" {
			continue
		}
		if o.OSArch == nil {
			o.OSArch = make(map[string]T)
		}
		o.OSArch[k] = v
	}
}

// UnmarshalJSON handles both scalar and object (map) forms.
// If JSON value is a scalar, it becomes the Default.
// If JSON value is an object, keys are OS/arch identifiers or "default".
func (o *Overrideable[T]) UnmarshalJSON(data []byte) error {
	var scalar T
	if err := json.Unmarshal(data, &scalar); err == nil {
		o.Default = scalar
		return nil
	}

	var m map[string]T
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("expected scalar or object: %w", err)
	}

	o.fromMap(m)
	return nil
}

// MarshalJSON emits a scalar when there are no OS/arch overrides,
// or an object with "default" and OS/arch keys when overrides exist.
func (o Overrideable[T]) MarshalJSON() ([]byte, error) {
	if len(o.OSArch) == 0 {
		return json.Marshal(o.Default)
	}
	m := make(map[string]T, len(o.OSArch)+1)
	m["default"] = o.Default
	for k, v := range o.OSArch {
		m[k] = v
	}
	return json.Marshal(m)
}
