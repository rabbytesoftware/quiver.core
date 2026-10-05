package models

import domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"

type EventKind string

const (
	EventKindStepStarted   EventKind = "step.started"
	EventKindStepCompleted EventKind = "step.completed"
	EventKindStepFailed    EventKind = "step.failed"
	EventKindPID           EventKind = "pid"
	EventKindEnded         EventKind = "ended"
	EventKindSurface       EventKind = "surface"
	// EventKindSurfaceClosed says the run that opened the surface has ended.
	EventKindSurfaceClosed EventKind = "surface.closed"
)

type Event struct {
	Kind      EventKind
	StepIndex int
	PID       int
	Err       error
	Note      string
	Outcome   domainRuntime.ExecutionOutcome
	// Surface is set on EventKindSurface: the interface the execution opened.
	Surface *domainRuntime.Surface
}
