package commands

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// SetSurface records the interface the current execution opened, or updates
// it (for example when the readiness probe flips Ready).
type SetSurface struct {
	Namespace   domain.Namespace
	ExecutionID string
	Surface     domainRuntime.Surface
}

func (c SetSurface) AggregateID() string {
	return c.Namespace.String()
}

func (c SetSurface) EventName() string {
	return "runtime.surface_set." + c.Namespace.String()
}

// ShouldSnapshot: the snapshot is a single upserted row under asynx v0.8.
func (c SetSurface) ShouldSnapshot() bool {
	return true
}

func (c SetSurface) Validate(current *domainRuntime.ArrowRuntime) error {
	return requireCurrentExecution("set surface", current, c.ExecutionID)
}

func (c SetSurface) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	var exec *domainRuntime.Execution
	if current.Execution != nil {
		copy := *current.Execution
		surface := c.Surface
		copy.Surface = &surface
		exec = &copy
	}
	return domainRuntime.ArrowRuntime{
		Ref:            current.Ref,
		State:          current.State,
		Execution:      exec,
		LastReturn:     current.LastReturn,
		PendingDepSync: current.PendingDepSync,
	}
}
