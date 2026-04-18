package step

type Step interface {
	Type() StepType
	Title() string
	ExitOnFailure() bool
	Resolve(os string) Step
}
