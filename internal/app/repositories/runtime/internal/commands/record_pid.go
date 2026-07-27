package commands

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

type RecordPID struct {
	Namespace   domain.Namespace
	ExecutionID string
	PID         int
}

func (c RecordPID) AggregateID() string {
	return c.Namespace.String()
}

func (c RecordPID) EventName() string {
	return "runtime.pid_recorded." + c.Namespace.String()
}

// ShouldSnapshot is true even though this is a high-frequency command. Under
// asynx v0.8 a snapshot is a single upserted row, not an appended one, so
// snapshotting every PID record costs O(1) per write instead of making every
// future read slower.
func (c RecordPID) ShouldSnapshot() bool {
	return true
}

func (c RecordPID) Validate(current *domainRuntime.ArrowRuntime) error {
	return requireCurrentExecution("record pid", current, c.ExecutionID)
}

func (c RecordPID) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	var exec *domainRuntime.Execution
	if current.Execution != nil {
		copy := *current.Execution
		copy.PID = c.PID
		exec = &copy
	}
	return domainRuntime.ArrowRuntime{
		Ref:        current.Ref,
		State:      current.State,
		Execution:  exec,
		LastReturn: current.LastReturn,
	}
}
