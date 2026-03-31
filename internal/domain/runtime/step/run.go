package step

import "time"

type RunStep struct {
	Title         string             `yaml:"title"          json:"title"`
	ExitOnFailure bool               `yaml:"exit_on_failure" json:"exit_on_failure"`
	Command       OverrideableString `yaml:"command"         json:"command"`
	Timeout       OverrideableString `yaml:"timeout"         json:"timeout"`
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
		Title:         title,
		ExitOnFailure: exitOnFailure,
		Command:       OverrideableString{Default: command},
		Timeout:       OverrideableString{Default: timeoutStr},
	}
}

func (s RunStep) Type() StepType { return StepTypeRun }
