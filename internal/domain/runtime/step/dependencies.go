package step

import "encoding/json"

type DependenciesStep struct {
	BasicStep
}

func NewDependenciesStep(
	title string,
) DependenciesStep {
	return DependenciesStep{
		BasicStep: BasicStep{stepType: StepTypeDependencies, exitOnFailure: true, title: title},
	}
}

func (s DependenciesStep) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type          StepType `json:"type"`
		Title         string   `json:"title"`
		ExitOnFailure bool     `json:"exit_on_failure"`
	}{
		Type:          s.stepType,
		Title:         s.title,
		ExitOnFailure: s.exitOnFailure,
	})
}
