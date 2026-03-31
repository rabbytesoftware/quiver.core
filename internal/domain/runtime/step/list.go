package step

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// StepList is a list of heterogeneous step types.
// It implements custom YAML and JSON marshaling/unmarshaling to handle the type discriminator.
type StepList []Step

// stepFactories maps step type strings to constructor functions.
var stepFactories = map[string]func() Step{
	string(StepTypeRun):          func() Step { return &RunStep{} },
	string(StepTypeFetch):        func() Step { return &FetchStep{} },
	string(StepTypeSignal):       func() Step { return &SignalStep{} },
	string(StepTypeDependencies): func() Step { return &DependenciesStep{} },
}

// decodeStep resolves the factory for a given type string and returns a zero-value Step.
func decodeStep(typeStr string) (Step, error) {
	factory, ok := stepFactories[typeStr]
	if !ok {
		return nil, fmt.Errorf("unknown step type %q", typeStr)
	}
	return factory(), nil
}

// marshalStepToJSON marshals a single step to JSON, injecting the "type" discriminator field.
// The "type" is not a struct field on concrete step types — it is returned by the Type() method.
func marshalStepToJSON(s Step) (json.RawMessage, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal step: %w", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to process step fields: %w", err)
	}

	typeJSON, err := json.Marshal(string(s.Type()))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal step type: %w", err)
	}

	m["type"] = typeJSON

	return json.Marshal(m)
}

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

		s, err := decodeStep(probe.Type)
		if err != nil {
			return err
		}

		if err := node.Decode(s); err != nil {
			return fmt.Errorf("failed to decode %q step: %w", probe.Type, err)
		}

		*l = append(*l, s)
	}

	return nil
}

// MarshalYAML marshals each step to YAML, injecting the "type" field.
// Since Step is an interface, we marshal to JSON first, then convert to map for YAML.
func (l StepList) MarshalYAML() (interface{}, error) {
	items := make([]map[string]interface{}, len(l))
	for i, s := range l {
		// Marshal step to JSON with type injected
		raw, err := marshalStepToJSON(s)
		if err != nil {
			return nil, err
		}

		// Convert JSON to map for YAML marshaling
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("failed to convert step to map: %w", err)
		}

		items[i] = m
	}

	return items, nil
}

// UnmarshalJSON handles unmarshaling a JSON array into a StepList,
// dispatching to the correct concrete step type based on the "type" field.
func (l *StepList) UnmarshalJSON(data []byte) error {
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return fmt.Errorf("expected JSON array of steps: %w", err)
	}

	for _, raw := range raws {
		var probe struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			return fmt.Errorf("failed to read step type: %w", err)
		}

		s, err := decodeStep(probe.Type)
		if err != nil {
			return err
		}

		if err := json.Unmarshal(raw, s); err != nil {
			return fmt.Errorf("failed to decode %q step: %w", probe.Type, err)
		}

		*l = append(*l, s)
	}

	return nil
}

// MarshalJSON marshals each step to JSON, injecting the "type" discriminator field.
func (l StepList) MarshalJSON() ([]byte, error) {
	items := make([]json.RawMessage, len(l))
	for i, s := range l {
		raw, err := marshalStepToJSON(s)
		if err != nil {
			return nil, err
		}
		items[i] = raw
	}
	return json.Marshal(items)
}
