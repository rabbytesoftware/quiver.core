package step

type StepType string

const (
	StepTypeRun          StepType = "run"
	StepTypeFetch        StepType = "fetch"
	StepTypeSignal       StepType = "signal"
	StepTypeDependencies StepType = "dependencies"
)

// Step is the interface for all step types.
// Each concrete step type implements Type() to return its discriminator.
// Fields like Title and ExitOnFailure are accessed directly on the concrete type
// after a type switch, not through interface methods.
type Step interface {
	Type() StepType
}
