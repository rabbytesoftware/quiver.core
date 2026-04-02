package step

import (
	"encoding/json"
	"time"
)

type SignalStep struct {
	Kind          StepType `json:"type"`
	Title         string   `json:"title"`
	exitOnFailure bool
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
		exitOnFailure: exitOnFailure,
		Signal:        Overrideable[string]{Default: signal},
		Timeout:       Overrideable[string]{Default: timeoutStr},
	}
}

func (s SignalStep) Type() StepType      { return s.Kind }
func (s SignalStep) ExitOnFailure() bool { return s.exitOnFailure }

func (s SignalStep) MarshalJSON() ([]byte, error) {
	type wire struct {
		Kind          StepType             `json:"type"`
		Title         string               `json:"title"`
		ExitOnFailure bool                 `json:"exit_on_failure"`
		Signal        Overrideable[string] `json:"signal"`
		Timeout       Overrideable[string] `json:"timeout"`
	}
	return json.Marshal(wire{
		Kind:          s.Kind,
		Title:         s.Title,
		ExitOnFailure: s.exitOnFailure,
		Signal:        s.Signal,
		Timeout:       s.Timeout,
	})
}

func (s *SignalStep) UnmarshalJSON(data []byte) error {
	var wire struct {
		Kind          StepType             `json:"type"`
		Title         string               `json:"title"`
		ExitOnFailure bool                 `json:"exit_on_failure"`
		Signal        Overrideable[string] `json:"signal"`
		Timeout       Overrideable[string] `json:"timeout"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	s.Kind = wire.Kind
	s.Title = wire.Title
	s.exitOnFailure = wire.ExitOnFailure
	s.Signal = wire.Signal
	s.Timeout = wire.Timeout
	return nil
}
