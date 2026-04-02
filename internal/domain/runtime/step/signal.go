package step

import "time"

type SignalStep struct {
	Kind          StepType             `json:"type"`
	Title         string               `json:"title"`
	ExitOnFailure bool                 `json:"exit_on_failure"`
	Signal        Overrideable[string] `json:"signal"`
	Timeout       Overrideable[string] `json:"timeout"`
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
		Kind:          StepTypeSignal,
		Title:         title,
		ExitOnFailure: exitOnFailure,
		Signal:        Overrideable[string]{Default: signal},
		Timeout:       Overrideable[string]{Default: timeoutStr},
	}
}

func (s SignalStep) Type() StepType { return s.Kind }
