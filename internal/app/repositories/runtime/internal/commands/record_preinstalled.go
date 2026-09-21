package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// RecordPreinstalled lands an arrow at Ready without an install ever having
// run. Two callers use it: an ordinary preinstalled detection, where nothing
// ran and LastReturn stays nil, and a catalog swap raised after an arrow's
// own update already succeeded (domain.Arrow.AlreadyReady; see
// onArrowUpgraded), which sets LastReturn to carry that outcome onto a brand
// new aggregate that never ran anything itself.
//
// Ready is an accepted current state, alongside no-aggregate and Absent, so
// the command stays idempotent: Add writes the runtime before the catalog
// row, so a failed Add can leave a Ready runtime the next Add must converge
// on rather than trip over.
type RecordPreinstalled struct {
	Namespace domain.Namespace
	// LastReturn is set only by the catalog-swap caller.
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
