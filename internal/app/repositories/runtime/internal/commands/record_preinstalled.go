package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// RecordPreinstalled lands an arrow at Ready without an install ever having
// run, keeping whatever return history the aggregate already had.
//
// Ready is an accepted current state, alongside no-aggregate and Absent, so
// the command stays idempotent: Add writes the runtime before the catalog
// row, so a failed Add can leave a Ready runtime the next Add must converge
// on rather than trip over.
type RecordPreinstalled struct {
	Namespace domain.Namespace
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
		Ref:   c.Namespace,
		State: domain.ArrowStateReady,
	}
	if current == nil {
		return next
	}

	next.LastReturn = current.LastReturn
	next.PendingDepSync = current.PendingDepSync

	return next
}
