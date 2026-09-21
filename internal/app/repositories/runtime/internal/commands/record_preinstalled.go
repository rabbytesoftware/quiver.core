package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// RecordPreinstalled lands an arrow at Ready without an install ever having
// run. It is the one command that can create a runtime aggregate directly at
// Ready: every other route into Ready passes through an execution that ended.
//
// Two callers use it, for two different reasons:
//
//   - A preinstalled lifecycle detection (the ordinary case): nothing ran, so
//     there is no return to record, and LastReturn is left nil -- a caller
//     reading it on a preinstalled arrow correctly sees that this machine has
//     never executed anything for it.
//   - A catalog swap raised after an arrow's own update: execution already
//     succeeded (domain.Arrow.AlreadyReady; see onArrowUpgraded in
//     usecases/runtime.go): the swapped-to namespace is a brand new
//     aggregate that never ran anything itself, but the software it
//     represents was just updated, and a caller should still be able to see
//     what that update did. LastReturn carries that outcome through
//     explicitly, since there is no current aggregate at this id to inherit
//     it from.
//
// The accepted current states are the three that all mean "Quiver does not
// consider this installed, and is not in the middle of anything": no aggregate
// at all (the ordinary first-detection case), Absent, and Ready. Ready is
// accepted so the command is idempotent — Add writes the runtime before the
// catalog row, so an Add that fails afterwards leaves a Ready runtime behind
// that the next Add must be able to converge on rather than trip over.
type RecordPreinstalled struct {
	Namespace domain.Namespace
	// LastReturn is nil for an ordinary preinstalled detection. Set only by
	// the catalog-swap caller, to carry an already-completed update's outcome
	// onto the new aggregate it could otherwise never reach.
	LastReturn *domainRuntime.Return
}

func (c RecordPreinstalled) AggregateID() string {
	return c.Namespace.String()
}

func (c RecordPreinstalled) EventName() string {
	return "runtime.preinstalled." + c.Namespace.String()
}

func (c RecordPreinstalled) ShouldSnapshot() bool {
	return true
}

func (c RecordPreinstalled) Validate(current *domainRuntime.ArrowRuntime) error {
	if current == nil || current.Ref == "" {
		return nil
	}
	if current.Execution != nil {
		return fmt.Errorf("record preinstalled: %w", asynxModels.ErrValidation)
	}
	if current.State != domain.ArrowStateAbsent && current.State != domain.ArrowStateReady {
		return fmt.Errorf("record preinstalled: %w", asynxModels.ErrValidation)
	}

	return nil
}

func (c RecordPreinstalled) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	next := domainRuntime.ArrowRuntime{
		Ref:        c.Namespace,
		State:      domain.ArrowStateReady,
		LastReturn: c.LastReturn,
	}
	if current == nil {
		return next
	}

	if next.LastReturn == nil {
		next.LastReturn = current.LastReturn
	}
	next.PendingDepSync = current.PendingDepSync

	return next
}
