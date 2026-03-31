package step

import "time"

type SignalStep struct {
	Title         string             `yaml:"title"          json:"title"`
	ExitOnFailure bool               `yaml:"exit_on_failure" json:"exit_on_failure"`
	Signal        OverrideableString `yaml:"signal"         json:"signal"`
	Timeout       OverrideableString `yaml:"timeout"        json:"timeout"`
}

// NewSignalStep creates a SignalStep with the given values.
func NewSignalStep(
	title string,
	signal string,
	timeout time.Duration,
	exitOnFailure bool,
) SignalStep {
	timeoutStr := ""
	if timeout > 0 {
		timeoutStr = timeout.String()
	}
	return SignalStep{
		Title:         title,
		ExitOnFailure: exitOnFailure,
		Signal:        OverrideableString{Default: signal},
		Timeout:       OverrideableString{Default: timeoutStr},
	}
}

func (s SignalStep) Type() StepType { return StepTypeSignal }
