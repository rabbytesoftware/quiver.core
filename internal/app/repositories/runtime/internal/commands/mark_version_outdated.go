package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// MarkVersionOutdated moves an arrow to Outdated because a passive version
// check found a newer release of it upstream. That is a different fact from
// the one MarkOutdated records, which is that the arrow's own dependency graph
// changed and needs re-syncing: MarkOutdated always writes a PendingDepSync,
// and an empty-but-present one reads downstream as "a dependency sync is
// pending and changes nothing" rather than "no dependency sync is pending" —
// a distinction runtimeUsecase.syncDeps branches on directly. So this command
// neither invents a PendingDepSync nor destroys one, it carries through
// whatever the aggregate already had.
//
// Unlike MarkOutdated it also refuses a namespace with no runtime aggregate.
// MarkOutdated can be sent only by callers that have already established the
// arrow is installed and Ready; a version check has no such luxury, because
// GetDetail runs it for any namespace it is asked about — including arrows
// catalogued but never installed, and namespaces with no catalog row at all.
// An arrow nobody installed must not grow a runtime just by being looked at.
//
// Outdated is accepted as a current state so the command converges rather than
// trips: a later check that finds a different recommended ref sends this again
// while the aggregate already sits where it wants it.
type MarkVersionOutdated struct {
	Namespace domain.Namespace
}

func (c MarkVersionOutdated) AggregateID() string {
	return c.Namespace.String()
}

// EventName deliberately shares MarkOutdated's topic. Five commands already
// share runtime.begun.*, and a subscriber's interest here is the same either
// way: the arrow is outdated and clients need to hear about it. Sharing it is
// what makes the hub broadcast already wired to OnRuntimeOutdated carry this
// to the frontend with no new wiring.
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
