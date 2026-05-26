package v0

import "gopkg.in/yaml.v3"

type quiverV0 struct {
	Schema   string         `yaml:"schema"`
	Metadata metadataV0     `yaml:"metadata"`
	Arrows   []arrowEntryV0 `yaml:"arrows"`
}

type metadataV0 struct {
	Name        string   `yaml:"name"`
	Version     string   `yaml:"version"`
	Description string   `yaml:"description"`
	URL         string   `yaml:"url"`
	Maintainers []string `yaml:"maintainers"`
	Tags        []string `yaml:"tags"`
	Media       mediaV0  `yaml:"media"`
}

type mediaV0 struct {
	Icon   string `yaml:"icon"`
	Banner string `yaml:"banner"`
}

type arrowEntryV0 struct {
	Path      string
	Namespace string
}

func (e *arrowEntryV0) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		e.Namespace = value.Value
		return nil
	}
	type alias struct {
		Path      string `yaml:"path"`
		Namespace string `yaml:"namespace"`
	}
	var a alias
	if err := value.Decode(&a); err != nil {
		return err
	}
	e.Path = a.Path
	e.Namespace = a.Namespace
	return nil
}
