package step

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
	domainstep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
)

// Request carries the execution context each handler may need.
type Request struct {
	NSKey   string
	WorkDir string
	Vars    map[string]string
	// OSArch is the current host platform (e.g. "darwin/arm64") used to resolve
	// Overrideable step fields to their platform-specific values.
	OSArch domain.OS
	// Tracker is set by the wizard for handlers that start or signal OS processes.
	// Nil for step types that do not manage processes (e.g. download).
	Tracker ProcessTracker
}

// Handler is the single port every step adapter must implement.
// ShouldExecute declares which StepType this handler owns.
// Execute carries out the step; the handler type-asserts s internally.
type Handler[S any] interface {
	ShouldExecute(
		t domainstep.StepType,
	) bool

	Execute(
		ctx context.Context,
		req Request,
		s S,
	) error
}
