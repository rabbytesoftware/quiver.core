package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

type RecordPID struct {
	Namespace domain.Namespace
	PID       int
}

func (c RecordPID) AggregateID() string {
	return c.Namespace.String()
}

func (c RecordPID) EventName() string {
	return "runtime.pid_recorded." + c.Namespace.String()
}

func (c RecordPID) ShouldSnapshot() bool {
	return false
}

func (c RecordPID) Validate(current *domainRuntime.ArrowRuntime) error {
	if current == nil || current.Ref == "" {
		return fmt.Errorf("record pid: %w", asynxModels.ErrValidation)
	}
	if current.Execution == nil {
		return fmt.Errorf("record pid: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c RecordPID) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	var exec *domainRuntime.Execution
	if current.Execution != nil {
		copy := *current.Execution
		copy.PID = c.PID
		exec = &copy
	}
	return domainRuntime.ArrowRuntime{
		Ref:        current.Ref,
		State:      current.State,
		Execution:  exec,
		LastReturn: current.LastReturn,
	}
}
