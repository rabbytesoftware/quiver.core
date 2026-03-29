package step

import (
	"encoding/json"
	"time"
)

type RunStep struct {
	BasicStep
	Command string        `json:"command"`
	Timeout time.Duration `json:"timeout"`
}

func NewRunStep(
	title string,
	command string,
	timeout time.Duration,
	exitOnFailure bool,
) RunStep {
	return RunStep{
		BasicStep: BasicStep{stepType: StepTypeRun, exitOnFailure: exitOnFailure, title: title},
		Command:   command,
		Timeout:   timeout,
	}
}

func (s RunStep) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type          StepType      `json:"type"`
		Title         string        `json:"title"`
		ExitOnFailure bool          `json:"exit_on_failure"`
		Command       string        `json:"command"`
		Timeout       time.Duration `json:"timeout"`
	}{
		Type:          s.stepType,
		Title:         s.title,
		ExitOnFailure: s.exitOnFailure,
		Command:       s.Command,
		Timeout:       s.Timeout,
	})
}
