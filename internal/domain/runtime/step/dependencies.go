package step

type DependenciesStep struct {
	Title string `yaml:"title" json:"title"`
}

// NewDependenciesStep creates a DependenciesStep with the given title.
func NewDependenciesStep(title string) DependenciesStep {
	return DependenciesStep{
		Title: title,
	}
}

func (s DependenciesStep) Type() StepType { return StepTypeDependencies }
