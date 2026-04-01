package step

import "time"

type RunStep struct {
	Kind          StepType             `json:"type"`
	Title         string               `json:"title"`
	ExitOnFailure bool                 `json:"exit_on_failure"`
	Command       Overrideable[string] `json:"command"`
	Timeout       Overrideable[string] `json:"timeout"`
}

// NewRunStep creates a RunStep with the given values.
// Timeout is converted to a string using time.Duration.String().
func NewRunStep(
	title string,
	command string,
	timeout time.Duration,
	exitOnFailure bool,
) RunStep {
	timeoutStr := ""
	if timeout > 0 {
		timeoutStr = timeout.String()
	}
	return RunStep{
		Kind:          StepTypeRun,
		Title:         title,
		ExitOnFailure: exitOnFailure,
		Command:       Overrideable[string]{Default: command},
		Timeout:       Overrideable[string]{Default: timeoutStr},
	}
}

func (s RunStep) Type() StepType { return s.Kind }
