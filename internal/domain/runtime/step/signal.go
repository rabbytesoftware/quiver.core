package step

import "encoding/json"

type SignalKind string

const (
	SignalKindGraceful  SignalKind = "graceful"
	SignalKindKill      SignalKind = "kill"
	SignalKindInterrupt SignalKind = "interrupt"
)

type SignalStep struct {
	BasicStep
	Signal  Overrideable[SignalKind] `json:"signal"`
	Timeout Overrideable[string]     `json:"timeout"`
}

func (s SignalStep) Resolve(os string) Step {
	s.Signal = Overrideable[SignalKind]{Default: s.Signal.Resolve(os)}
	s.Timeout = Overrideable[string]{Default: s.Timeout.Resolve(os)}
	return s
}

func NewSignalStep(
	title string,
	signal SignalKind,
	timeout string,
	exitOnFailure bool,
) SignalStep {
	return SignalStep{
		BasicStep: newBasicStep(StepTypeSignal, title, exitOnFailure),
		Signal:    Overrideable[SignalKind]{Default: signal},
		Timeout:   Overrideable[string]{Default: timeout},
	}
}

func (s SignalStep) MarshalJSON() ([]byte, error) {
	type wire struct {
		Kind          StepType                 `json:"type"`
		Title         string                   `json:"title"`
		ExitOnFailure bool                     `json:"exit_on_failure"`
		Signal        Overrideable[SignalKind] `json:"signal"`
		Timeout       Overrideable[string]     `json:"timeout"`
	}
	return json.Marshal(wire{
		Kind:          s.Type(),
		Title:         s.Title(),
		ExitOnFailure: s.ExitOnFailure(),
		Signal:        s.Signal,
		Timeout:       s.Timeout,
	})
}

func (s *SignalStep) UnmarshalJSON(data []byte) error {
	var wire struct {
		Kind          StepType                 `json:"type"`
		Title         string                   `json:"title"`
		ExitOnFailure bool                     `json:"exit_on_failure"`
		Signal        Overrideable[SignalKind] `json:"signal"`
		Timeout       Overrideable[string]     `json:"timeout"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	s.BasicStep = newBasicStep(wire.Kind, wire.Title, wire.ExitOnFailure)
	s.Signal = wire.Signal
	s.Timeout = wire.Timeout
	return nil
}
