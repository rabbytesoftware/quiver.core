package step

import "github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/process"

// ProcessTracker is the narrow interface step handlers use to record and
// retrieve the OS process for the current execution.
// The wizard owns the concrete implementation and passes it on each Request.
type ProcessTracker interface {
	// SetProcess records the process after it has been started.
	SetProcess(proc process.Process)

	// GetProcess returns the process and true if one has been set,
	// or nil, false if no process has been started yet.
	GetProcess() (process.Process, bool)
}
