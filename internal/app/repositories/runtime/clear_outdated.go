package runtime

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

type clearOutdatedCmd struct{ ns domain.Namespace }

func (c clearOutdatedCmd) AggregateID() string  { return c.ns.String() }
func (c clearOutdatedCmd) EventName() string    { return "runtime.outdated_cleared." + c.ns.String() }
func (c clearOutdatedCmd) ShouldSnapshot() bool { return true }

func (c clearOutdatedCmd) Validate(current *domainRuntime.ArrowRuntime) error {
	if current == nil || current.State != domain.ArrowStateOutdated {
		return fmt.Errorf("clear outdated: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c clearOutdatedCmd) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:        c.ns,
		State:      domain.ArrowStateReady,
		LastReturn: current.LastReturn,
	}
}
