package metadata

import (
	"fmt"
	"runtime"

	yaml "gopkg.in/yaml.v3"
)

// OsValue is a value that can be overridden per operating system.
// In YAML it accepts either a scalar (default only) or a map with a "default"
// key plus optional OS keys ("windows", "linux", "darwin").
type OsValue[T any] struct {
	Default T
	OS      map[string]T
}

// Resolve returns the OS-specific value for runtime.GOOS if present,
// otherwise returns Default.
func (o OsValue[T]) Resolve() T {
	if v, ok := o.OS[runtime.GOOS]; ok {
		return v
	}
	return o.Default
}

// UnmarshalYAML accepts either a scalar (sets Default) or a map with a "default"
// key plus optional OS keys ("windows", "linux", "darwin").
func (o *OsValue[T]) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		return value.Decode(&o.Default)
	}
	if value.Kind == yaml.MappingNode { //nolint:nestif
		var m map[string]T
		if err := value.Decode(&m); err != nil {
			return err
		}
		o.Default = m["default"]
		for k, v := range m {
			if k == "default" {
				continue
			}
			if o.OS == nil {
				o.OS = make(map[string]T)
			}
			o.OS[k] = v
		}
		return nil
	}
	return fmt.Errorf("osvalue: expected scalar or map, got node kind %v", value.Kind)
}
