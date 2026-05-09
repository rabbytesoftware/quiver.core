package step

import (
	"encoding/json"
	"fmt"
)

// StepList is a list of heterogeneous step types.
// It implements custom JSON marshaling/unmarshaling to handle the type discriminator.
type StepList []Step

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

		*l = append(*l, derefStep(s))
	}

	return nil
}

// derefStep converts pointer step types to their value form.
// Factories allocate pointers for JSON unmarshaling; this normalises the stored type.
func derefStep(s Step) Step {
	switch v := s.(type) {
	case *RunStep:
		return *v
	case *FetchStep:
		return *v
	case *SignalStep:
		return *v
	case *DependenciesStep:
		return *v
	default:
		return s
	}
}

// MarshalJSON marshals each step to JSON.
// Each concrete step type includes its Kind field as the "type" discriminator.
func (l StepList) MarshalJSON() ([]byte, error) {
	items := make([]json.RawMessage, len(l))
	for i, s := range l {
		raw, err := json.Marshal(s)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal step: %w", err)
		}
		items[i] = raw
	}
	return json.Marshal(items)
}
