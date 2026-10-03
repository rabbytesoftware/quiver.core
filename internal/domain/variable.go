package domain

import (
	"fmt"
	"slices"
)

const (
	MaxVariableNameLength = 255
)

// Built-in variable names. Each one is a fact Quiver computes about an
// execution — where it runs, what it runs as, which revision it came from — so
// none of them is a preference a caller can supply.
const (
	VarWorkdir        = "WORKDIR"
	VarInstallPath    = "INSTALL_PATH"
	VarArrowNamespace = "ARROW_NAMESPACE"
	VarPlatform       = "PLATFORM"
	VarRef            = "REF"
)

// Release-bound variable sources. A variable declaring one of them takes its
// value from the release its arrow's run is built from, not from the caller.
const (
	VarSourceReleaseAsset    = "release.asset"
	VarSourceReleaseChecksum = "release.checksum"
)

// ReservedVariableNames returns the built-in names. The order is fixed so a
// request setting several of them is always rejected on the same one.
func ReservedVariableNames() []string {
	return []string{
		VarWorkdir,
		VarInstallPath,
		VarArrowNamespace,
		VarPlatform,
		VarRef,
	}
}

// IsReservedVariable reports whether name is computed by Quiver.
func IsReservedVariable(
	name string,
) bool {
	return slices.Contains(ReservedVariableNames(), name)
}

type Variable struct {
	Name        string       `yaml:"name"        json:"name"`
	Description string       `yaml:"description" json:"description"`
	Default     string       `yaml:"default"     json:"default"`
	Values      []string     `yaml:"values"      json:"values"`
	Min         int          `yaml:"min"         json:"min"`
	Max         int          `yaml:"max"         json:"max"`
	Sensitive   bool         `yaml:"sensitive"   json:"sensitive"`
	Type        VariableType `yaml:"type"        json:"type"`
	From        string       `yaml:"from"        json:"from,omitempty"`
}

func (v *Variable) Validate() error {
	if v.Name == "" {
		return fmt.Errorf("variable name cannot be empty")
	}
	if len(v.Name) > MaxVariableNameLength {
		return fmt.Errorf("variable name exceeds max length of %d", MaxVariableNameLength)
	}

	if v.Max > 0 && v.Min > v.Max {
		return fmt.Errorf("variable min (%d) cannot be greater than max (%d)", v.Min, v.Max)
	}

	if v.From != "" && !slices.Contains(releaseSources(), v.From) {
		return fmt.Errorf("variable %q: unknown source %q", v.Name, v.From)
	}
	if v.From != "" && v.Default != "" {
		return fmt.Errorf("variable %q: a release-bound variable cannot have a default", v.Name)
	}

	if len(v.Values) > 0 && v.Default != "" && !slices.Contains(v.Values, v.Default) {
		return fmt.Errorf("default value '%s' not found in allowed values", v.Default)
	}

	return nil
}

func releaseSources() []string {
	return []string{VarSourceReleaseAsset, VarSourceReleaseChecksum}
}
