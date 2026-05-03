package models

import domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"

type EventKind string

const (
	EventKindStepStarted   EventKind = "step.started"
	EventKindStepCompleted EventKind = "step.completed"
	EventKindStepFailed    EventKind = "step.failed"
	EventKindPID           EventKind = "pid"
	EventKindEnded         EventKind = "ended"
)

type Event struct {
	Kind      EventKind
	StepIndex int
	PID       int
	Err       error
	Outcome   domainRuntime.ExecutionOutcome
}
