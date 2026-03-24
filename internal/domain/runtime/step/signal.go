package step

import "time"

type SignalStep struct {
	BasicStep
	Signal  string
	Timeout time.Duration
}

func NewSignalStep(title, signal string, timeout time.Duration, exitOnFailure bool) SignalStep {
	return SignalStep{
		BasicStep: BasicStep{stepType: StepTypeSignal, exitOnFailure: exitOnFailure, title: title},
		Signal:    signal,
		Timeout:   timeout,
	}
}
