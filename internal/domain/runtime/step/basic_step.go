package step

type BasicStep struct {
	stepType      StepType
	exitOnFailure bool
	title         string
}

func newBasicStep(
	stepType StepType,
	title string,
	exitOnFailure bool,
) BasicStep {
	return BasicStep{
		stepType:      stepType,
		exitOnFailure: exitOnFailure,
		title:         title,
	}
}

func (bs BasicStep) Type() StepType      { return bs.stepType }
func (bs BasicStep) Title() string       { return bs.title }
func (bs BasicStep) ExitOnFailure() bool { return bs.exitOnFailure }
