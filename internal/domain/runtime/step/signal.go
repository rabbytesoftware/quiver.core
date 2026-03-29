package step

import (
	"encoding/json"
	"time"
)

type SignalStep struct {
	BasicStep
	Signal  string        `json:"signal"`
	Timeout time.Duration `json:"timeout"`
}

func NewSignalStep(
	title string,
	signal string,
	timeout time.Duration,
	exitOnFailure bool,
) SignalStep {
	return SignalStep{
		BasicStep: BasicStep{stepType: StepTypeSignal, exitOnFailure: exitOnFailure, title: title},
		Signal:    signal,
		Timeout:   timeout,
	}
}

func (s SignalStep) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type          StepType      `json:"type"`
		Title         string        `json:"title"`
		ExitOnFailure bool          `json:"exit_on_failure"`
		Signal        string        `json:"signal"`
		Timeout       time.Duration `json:"timeout"`
	}{
		Type:          s.stepType,
		Title:         s.title,
		ExitOnFailure: s.exitOnFailure,
		Signal:        s.Signal,
		Timeout:       s.Timeout,
	})
}
