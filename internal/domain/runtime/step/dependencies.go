package step

type DependenciesStep struct {
	BasicStep
}

func NewDependenciesStep(title string) DependenciesStep {
	return DependenciesStep{
		BasicStep: BasicStep{stepType: StepTypeDependencies, exitOnFailure: true, title: title},
	}
}
