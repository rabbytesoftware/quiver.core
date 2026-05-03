package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

type ClearOutdated struct {
	Namespace domain.Namespace
}

func (c ClearOutdated) AggregateID() string  { return c.Namespace.String() }
func (c ClearOutdated) EventName() string    { return "runtime.outdated_cleared." + c.Namespace.String() }
func (c ClearOutdated) ShouldSnapshot() bool { return true }

func (c ClearOutdated) Validate(current *domainRuntime.ArrowRuntime) error {
	if current == nil || current.State != domain.ArrowStateOutdated {
		return fmt.Errorf("clear outdated: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c ClearOutdated) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:            c.Namespace,
		State:          domain.ArrowStateReady,
		Execution:      nil,
		LastReturn:     current.LastReturn,
		PendingDepSync: nil,
	}
}
