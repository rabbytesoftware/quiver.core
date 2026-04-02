package step

type StepType string

const (
	StepTypeRun          StepType = "run"
	StepTypeFetch        StepType = "fetch"
	StepTypeSignal       StepType = "signal"
	StepTypeDependencies StepType = "dependencies"
)

// Step is the interface for all step types.
type Step interface {
	// Type returns the discriminator used to route the step to the correct handler.
	Type() StepType
	// ExitOnFailure reports whether a failure in this step should abort the execution.
	ExitOnFailure() bool
}
