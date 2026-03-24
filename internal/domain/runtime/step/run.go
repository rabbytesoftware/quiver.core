package step

import "time"

type RunStep struct {
	BasicStep
	Command string
	Timeout time.Duration
}

func NewRunStep(title, command string, timeout time.Duration, exitOnFailure bool) RunStep {
	return RunStep{
		BasicStep: BasicStep{stepType: StepTypeRun, exitOnFailure: exitOnFailure, title: title},
		Command:   command,
		Timeout:   timeout,
	}
}
