package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

type MarkOutdated struct {
	Namespace   domain.Namespace
	AddedDeps   []domain.Namespace
	RemovedDeps []domain.Namespace
}

func (c MarkOutdated) AggregateID() string  { return c.Namespace.String() }
func (c MarkOutdated) EventName() string    { return "runtime.outdated." + c.Namespace.String() }
func (c MarkOutdated) ShouldSnapshot() bool { return true }

func (c MarkOutdated) Validate(current *domainRuntime.ArrowRuntime) error {
	if current != nil && current.State != domain.ArrowStateReady {
		return fmt.Errorf("mark outdated: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c MarkOutdated) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	rt := domainRuntime.ArrowRuntime{
		Ref:   c.Namespace,
		State: domain.ArrowStateOutdated,
		PendingDepSync: &domainRuntime.DepSyncInfo{
			AddedDeps:   c.AddedDeps,
			RemovedDeps: c.RemovedDeps,
		},
	}
	if current != nil {
		rt.LastReturn = current.LastReturn
	}
	return rt
}
