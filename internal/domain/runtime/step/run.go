package step

import "encoding/json"

type RunStep struct {
	BasicStep
	Command  Overrideable[string] `json:"command"`
	Elevated Overrideable[bool]   `json:"elevated"`
	Timeout  Overrideable[string] `json:"timeout"`
	// UI, when set, is the interface that exists exactly while this run's
	// process runs.
	UI *UIOptions `json:"ui,omitempty"`
}

func (s RunStep) Resolve(os string) Step {
	s.Command = Overrideable[string]{Default: s.Command.Resolve(os)}
	s.Elevated = Overrideable[bool]{Default: s.Elevated.Resolve(os)}
	s.Timeout = Overrideable[string]{Default: s.Timeout.Resolve(os)}
	return s
}

func NewRunStep(
	title string,
	command string,
	elevated bool,
	timeout string,
	exitOnFailure bool,
) RunStep {
	return RunStep{
		BasicStep: newBasicStep(StepTypeRun, title, exitOnFailure),
		Command:   Overrideable[string]{Default: command},
		Elevated:  Overrideable[bool]{Default: elevated},
		Timeout:   Overrideable[string]{Default: timeout},
	}
}

// WithUI returns the step with ui as the interface it opens while it runs.
func (s RunStep) WithUI(
	ui UIOptions,
) RunStep {
	s.UI = &ui
	return s
}

func (s RunStep) MarshalJSON() ([]byte, error) {
	type wire struct {
		Kind          StepType             `json:"type"`
		Title         string               `json:"title"`
		ExitOnFailure bool                 `json:"exit_on_failure"`
		Command       Overrideable[string] `json:"command"`
		Elevated      Overrideable[bool]   `json:"elevated"`
		Timeout       Overrideable[string] `json:"timeout"`
		UI            *UIOptions           `json:"ui,omitempty"`
	}
	return json.Marshal(wire{
		Kind:          s.Type(),
		Title:         s.Title(),
		ExitOnFailure: s.ExitOnFailure(),
		Command:       s.Command,
		Elevated:      s.Elevated,
		Timeout:       s.Timeout,
		UI:            s.UI,
	})
}

func (s *RunStep) UnmarshalJSON(data []byte) error {
	var wire struct {
		Kind          StepType             `json:"type"`
		Title         string               `json:"title"`
		ExitOnFailure bool                 `json:"exit_on_failure"`
		Command       Overrideable[string] `json:"command"`
		Elevated      Overrideable[bool]   `json:"elevated"`
		Timeout       Overrideable[string] `json:"timeout"`
		UI            *UIOptions           `json:"ui,omitempty"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	s.BasicStep = newBasicStep(wire.Kind, wire.Title, wire.ExitOnFailure)
	s.Command = wire.Command
	s.Elevated = wire.Elevated
	s.Timeout = wire.Timeout
	s.UI = wire.UI
	return nil
}
