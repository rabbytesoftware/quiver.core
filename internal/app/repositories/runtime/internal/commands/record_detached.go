package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

// RecordDetached transitions a running arrow to detached when its OS process
// survived a crash. The arrow is alive but Quiver cannot monitor it — the user
// must stop and restart it through Quiver to restore monitoring.
type RecordDetached struct {
	Namespace domain.Namespace
}

func (c RecordDetached) AggregateID() string {
	return c.Namespace.String()
}

func (c RecordDetached) EventName() string {
	return "runtime.detached." + c.Namespace.String()
}

func (c RecordDetached) ShouldSnapshot() bool {
	return true
}

func (c RecordDetached) Validate(current *domainRuntime.ArrowRuntime) error {
	if current == nil || current.Ref == "" {
		return fmt.Errorf("record detached: %w", asynxModels.ErrValidation)
	}
	if !current.State.CanTransitionTo(domain.ArrowStateDetached) {
		return fmt.Errorf("record detached: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c RecordDetached) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:        current.Ref,
		State:      domain.ArrowStateDetached,
		Execution:  nil,
		LastReturn: current.LastReturn,
	}
}
