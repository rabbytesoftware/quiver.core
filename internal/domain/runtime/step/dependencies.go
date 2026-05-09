package step

import "encoding/json"

type DependenciesStep struct {
	BasicStep
}

func (s DependenciesStep) Resolve(_ string) Step { return s }

func NewDependenciesStep(title string) DependenciesStep {
	return DependenciesStep{
		BasicStep: newBasicStep(StepTypeDependencies, title, true),
	}
}

func (s DependenciesStep) MarshalJSON() ([]byte, error) {
	type wire struct {
		Kind  StepType `json:"type"`
		Title string   `json:"title"`
	}
	return json.Marshal(wire{
		Kind:  s.Type(),
		Title: s.Title(),
	})
}

func (s *DependenciesStep) UnmarshalJSON(data []byte) error {
	var wire struct {
		Kind  StepType `json:"type"`
		Title string   `json:"title"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	s.BasicStep = newBasicStep(wire.Kind, wire.Title, true)
	return nil
}
