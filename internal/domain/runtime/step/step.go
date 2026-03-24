package step

type StepType string

const (
	StepTypeRun          StepType = "run"
	StepTypeFetch        StepType = "fetch"
	StepTypeSignal       StepType = "signal"
	StepTypeDependencies StepType = "dependencies"
)

type Step interface {
	Type() StepType
	Title() string
	ExitOnFailure() bool
}

// BasicStep holds common fields shared by all concrete step types.
// Fields are unexported — accessed through the Step interface methods.
type BasicStep struct {
	stepType      StepType
	exitOnFailure bool
	title         string
}

func (bs BasicStep) Type() StepType      { return bs.stepType }
func (bs BasicStep) Title() string       { return bs.title }
func (bs BasicStep) ExitOnFailure() bool { return bs.exitOnFailure }
