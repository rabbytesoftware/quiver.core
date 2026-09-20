package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// ClearVersionOutdated gives an arrow back when a later version check finds the
// drift is over and nobody ran an update to end it — upstream yanked the newer
// release, or a tracked branch was reverted. BeginUpdate covers the other
// direction out of Outdated, but only for a user who actually updates; nothing
// runs when the drift resolves by itself.
//
// This is about the badge, and only the badge. A version-drift Outdated is not
// a gate — BeginExecution admits it wherever it admits Ready — so an arrow left
// sitting there is still perfectly runnable. What it must not do is keep
// claiming an update is available when upstream no longer offers one.
//
// It refuses an Outdated that carries a PendingDepSync. That one was put there
// by a dependency-graph change, which a version check neither observed nor
// resolved, and clearing it would drop a re-sync the arrow genuinely still
// needs. This is the same distinction MarkVersionOutdated keeps on the way in,
// and the reason both exist separately from MarkOutdated.
type ClearVersionOutdated struct {
	Namespace domain.Namespace
}

func (c ClearVersionOutdated) AggregateID() string {
	return c.Namespace.String()
}

func (c ClearVersionOutdated) EventName() string {
	return "runtime.outdated_cleared." + c.Namespace.String()
}

func (c ClearVersionOutdated) ShouldSnapshot() bool {
	return true
}

func (c ClearVersionOutdated) Validate(current *domainRuntime.ArrowRuntime) error {
	if current == nil || current.Ref == "" {
		return fmt.Errorf("clear version outdated: %w", asynxModels.ErrValidation)
	}
	if current.Execution != nil {
		return fmt.Errorf("clear version outdated: %w", asynxModels.ErrValidation)
	}
	if current.State != domain.ArrowStateOutdated {
		return fmt.Errorf("clear version outdated: %w", asynxModels.ErrValidation)
	}
	if current.PendingDepSync != nil {
		return fmt.Errorf("clear version outdated: %w", asynxModels.ErrValidation)
	}

	return nil
}

func (c ClearVersionOutdated) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:        c.Namespace,
		State:      domain.ArrowStateReady,
		LastReturn: preserveLastReturn(current),
	}
}
