package step

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// StepList is a list of heterogeneous step types.
// It implements custom YAML unmarshaling to handle the type discriminator.
type StepList []Step

// UnmarshalYAML handles unmarshaling a YAML sequence into a StepList,
// dispatching to the correct concrete step type based on the "type" field.
func (l *StepList) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("expected sequence, got %v", value.Kind)
	}

	for _, node := range value.Content {
		var probe struct {
			Type string `yaml:"type"`
		}
		if err := node.Decode(&probe); err != nil {
			return fmt.Errorf("failed to read step type: %w", err)
		}

		var s Step
		switch probe.Type {
		case string(StepTypeRun):
			var r RunStep
			if err := node.Decode(&r); err != nil {
				return fmt.Errorf("failed to decode run step: %w", err)
			}
			s = r

		case string(StepTypeFetch):
			var f FetchStep
			if err := node.Decode(&f); err != nil {
				return fmt.Errorf("failed to decode fetch step: %w", err)
			}
			s = f

		case string(StepTypeSignal):
			var sig SignalStep
			if err := node.Decode(&sig); err != nil {
				return fmt.Errorf("failed to decode signal step: %w", err)
			}
			s = sig

		case string(StepTypeDependencies):
			var d DependenciesStep
			if err := node.Decode(&d); err != nil {
				return fmt.Errorf("failed to decode dependencies step: %w", err)
			}
			s = d

		default:
			return fmt.Errorf("unknown step type %q", probe.Type)
		}

		*l = append(*l, s)
	}

	return nil
}
