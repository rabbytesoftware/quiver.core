package step

type DependenciesStep struct {
	Kind  StepType `json:"type"`
	Title string   `json:"title"`
}

// NewDependenciesStep creates a DependenciesStep with the given title.
func NewDependenciesStep(title string) DependenciesStep {
	return DependenciesStep{
		Kind:  StepTypeDependencies,
		Title: title,
	}
}

func (s DependenciesStep) Type() StepType      { return s.Kind }
func (s DependenciesStep) ExitOnFailure() bool { return true }
