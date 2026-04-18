package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

type MarkStopping struct {
	Namespace domain.Namespace
}

func (c MarkStopping) AggregateID() string {
	return c.Namespace.String()
}

func (c MarkStopping) EventName() string {
	return "runtime.mark_stopping"
}

func (c MarkStopping) ShouldSnapshot() bool {
	return false
}

func (c MarkStopping) Validate(current *domainRuntime.ArrowRuntime) error {
	if current == nil || current.Ref == "" {
		return fmt.Errorf("mark stopping: %w", asynxModels.ErrValidation)
	}
	if current.State != domain.ArrowStateRunning {
		return fmt.Errorf("mark stopping: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c MarkStopping) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:        current.Ref,
		State:      domain.ArrowStateStopping,
		Execution:  current.Execution,
		LastReturn: current.LastReturn,
	}
}
