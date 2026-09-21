package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// MarkVersionOutdated moves an arrow to Outdated after a passive version
// check finds a newer release upstream, unlike MarkOutdated it preserves
// PendingDepSync rather than resetting it, and it refuses a namespace with no
// runtime aggregate since GetDetail runs this against arrows nobody installed.
type MarkVersionOutdated struct {
	Namespace domain.Namespace
}

func (c MarkVersionOutdated) AggregateID() string {
	return c.Namespace.String()
}

// EventName shares MarkOutdated's topic so the hub broadcast already wired
// to OnRuntimeOutdated carries this too, with no new wiring.
func (c MarkVersionOutdated) EventName() string {
	return "runtime.outdated." + c.Namespace.String()
}

func (c MarkVersionOutdated) ShouldSnapshot() bool {
	return true
}

func (c MarkVersionOutdated) Validate(current *domainRuntime.ArrowRuntime) error {
	if current == nil || current.Ref == "" {
		return fmt.Errorf("mark version outdated: %w", asynxModels.ErrValidation)
	}
	if current.Execution != nil {
		return fmt.Errorf("mark version outdated: %w", asynxModels.ErrValidation)
	}
	if current.State != domain.ArrowStateReady && current.State != domain.ArrowStateOutdated {
		return fmt.Errorf("mark version outdated: %w", asynxModels.ErrValidation)
	}

	return nil
}

func (c MarkVersionOutdated) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:            c.Namespace,
		State:          domain.ArrowStateOutdated,
		LastReturn:     preserveLastReturn(current),
		PendingDepSync: preservePendingDepSync(current),
	}
}

func preservePendingDepSync(current *domainRuntime.ArrowRuntime) *domainRuntime.DepSyncInfo {
	if current == nil {
		return nil
	}
	return current.PendingDepSync
}
