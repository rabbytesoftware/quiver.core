package commands

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// ClearSurface removes the interface the current execution had open, because
// the run step that opened it exited. The execution itself carries on.
type ClearSurface struct {
	Namespace   domain.Namespace
	ExecutionID string
}

func (c ClearSurface) AggregateID() string {
	return c.Namespace.String()
}

func (c ClearSurface) EventName() string {
	return "runtime.surface_cleared." + c.Namespace.String()
}

// ShouldSnapshot: the snapshot is a single upserted row under asynx v0.8.
func (c ClearSurface) ShouldSnapshot() bool {
	return true
}

func (c ClearSurface) Validate(
	current *domainRuntime.ArrowRuntime,
) error {
	return requireCurrentExecution("clear surface", current, c.ExecutionID)
}

func (c ClearSurface) EmitEvent(
	current *domainRuntime.ArrowRuntime,
) domainRuntime.ArrowRuntime {
	next := *current.Execution
	next.Surface = nil
	return domainRuntime.ArrowRuntime{
		Ref:            current.Ref,
		State:          current.State,
		Execution:      &next,
		LastReturn:     current.LastReturn,
		PendingDepSync: current.PendingDepSync,
	}
}
