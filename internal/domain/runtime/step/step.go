package step

type Step interface {
	Type() StepType
	Title() string
	ExitOnFailure() bool
	// Resolve flattens this step's Overrideable fields against os using
	// Overrideable.Resolve, which is an exact-key lookup. A step type that
	// carries Overrideable fields is resolved by the translator instead —
	// see resolveStepList, and Overrideable.Resolve's own doc for why glob
	// matching cannot live here. This stays for the step types that have
	// nothing to resolve, where it is the identity.
	Resolve(os string) Step
}
