package step

import (
	"encoding/json"
	"time"
)

type RunStep struct {
	Kind          StepType             `json:"type"`
	Title         string               `json:"title"`
	exitOnFailure bool                 `json:"exit_on_failure"`
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
		exitOnFailure: exitOnFailure,
		Command:       Overrideable[string]{Default: command},
		Timeout:       Overrideable[string]{Default: timeoutStr},
	}
}

func (s RunStep) Type() StepType      { return s.Kind }
func (s RunStep) ExitOnFailure() bool { return s.exitOnFailure }

func (s RunStep) MarshalJSON() ([]byte, error) {
	type wire struct {
		Kind          StepType             `json:"type"`
		Title         string               `json:"title"`
		ExitOnFailure bool                 `json:"exit_on_failure"`
		Command       Overrideable[string] `json:"command"`
		Timeout       Overrideable[string] `json:"timeout"`
	}
	return json.Marshal(wire{
		Kind:          s.Kind,
		Title:         s.Title,
		ExitOnFailure: s.exitOnFailure,
		Command:       s.Command,
		Timeout:       s.Timeout,
	})
}

func (s *RunStep) UnmarshalJSON(data []byte) error {
	var wire struct {
		Kind          StepType             `json:"type"`
		Title         string               `json:"title"`
		ExitOnFailure bool                 `json:"exit_on_failure"`
		Command       Overrideable[string] `json:"command"`
		Timeout       Overrideable[string] `json:"timeout"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	s.Kind = wire.Kind
	s.Title = wire.Title
	s.exitOnFailure = wire.ExitOnFailure
	s.Command = wire.Command
	s.Timeout = wire.Timeout
	return nil
}
