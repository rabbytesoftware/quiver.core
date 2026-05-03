package v0

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// overrideableV0 is the v0 manifest representation of a value that can be
// overridden per OS/arch. It unmarshals from either a scalar or a map:
//
//	command: "echo hello"           # scalar → Default only
//	url:
//	  default: "https://example.com/binary"
//	  linux/amd64: "https://example.com/linux-amd64"
type overrideableV0[T any] struct {
	Default T
	OSArch  map[string]T
}

func (o *overrideableV0[T]) UnmarshalYAML(value *yaml.Node) error {
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
			if o.OSArch == nil {
				o.OSArch = make(map[string]T)
			}
			o.OSArch[k] = v
		}
		return nil
	}
	return fmt.Errorf("expected scalar or map")
}

type arrowV0 struct {
	Schema    string              `yaml:"schema"`
	Metadata  metadataV0          `yaml:"metadata"`
	Variables []variableV0        `yaml:"variables"`
	Netbridge []portV0            `yaml:"netbridge"`
	Targets   map[string]targetV0 `yaml:"targets"`
}

type metadataV0 struct {
	Name        string     `yaml:"name"`
	Description string     `yaml:"description"`
	Version     string     `yaml:"version"`
	License     string     `yaml:"license"`
	URL         string     `yaml:"url"`
	Quiver      string     `yaml:"quiver"`
	Maintainers []creditV0 `yaml:"maintainers"`
	Credits     []creditV0 `yaml:"credits"`
	Media       mediaV0    `yaml:"media"`
	Tags        []string   `yaml:"tags"`
}

type creditV0 struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
	URL   string `yaml:"url"`
}

type mediaV0 struct {
	Icon   string `yaml:"icon"`
	Banner string `yaml:"banner"`
}

type targetV0 struct {
	Base         string                            `yaml:"base"`
	Requirements requirementsV0                    `yaml:"requirements"`
	Tools        []string                          `yaml:"tools"`
	Services     []string                          `yaml:"services"`
	Exports      map[string]overrideableV0[string] `yaml:"exports"`
	Lifecycle    lifecycleV0                       `yaml:"lifecycle"`
	Methods      map[string]methodV0               `yaml:"methods"`
}

type requirementsV0 struct {
	CpuCores *int `yaml:"cpu_cores"`
	RamGB    *int `yaml:"ram_gb"`
	DiskGB   *int `yaml:"disk_gb"`
}

type variableV0 struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Default     string   `yaml:"default"`
	Sensitive   bool     `yaml:"sensitive"`
	Values      []string `yaml:"values"`
	Min         int      `yaml:"min"`
	Max         int      `yaml:"max"`
	Type        string   `yaml:"type"`
}

type portV0 struct {
	Name     string `yaml:"name"`
	Protocol string `yaml:"protocol"`
	Default  int    `yaml:"default"`
	Required bool   `yaml:"required"`
}

type lifecycleV0 struct {
	Install   []stepV0 `yaml:"install"`
	Update    []stepV0 `yaml:"update"`
	Execute   []stepV0 `yaml:"execute"`
	Stop      []stepV0 `yaml:"stop"`
	Uninstall []stepV0 `yaml:"uninstall"`
}

type methodV0 struct {
	AvailableIn []string `yaml:"available_in"`
	Steps       []stepV0 `yaml:"steps"`
}

type stepV0 struct {
	Type          string                 `yaml:"type"`
	Title         string                 `yaml:"title"`
	Command       overrideableV0[string] `yaml:"command"`
	URL           overrideableV0[string] `yaml:"url"`
	To            overrideableV0[string] `yaml:"to"`
	Signal        overrideableV0[string] `yaml:"signal"`
	Elevated      overrideableV0[bool]   `yaml:"elevated"`
	Checksum      overrideableV0[string] `yaml:"checksum"`
	Timeout       overrideableV0[string] `yaml:"timeout"`
	ExitOnFailure *bool                  `yaml:"exit_on_failure"`
}
