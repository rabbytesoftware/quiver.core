package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

type BeginStop struct {
	Namespace domain.Namespace
	Steps     []domainStep.Step
	Variables map[string]string
	WorkDir   string
}

func (c BeginStop) AggregateID() string  { return c.Namespace.String() }
func (c BeginStop) EventName() string    { return "runtime.begun." + c.Namespace.String() }
func (c BeginStop) ShouldSnapshot() bool { return true }

func (c BeginStop) Validate(current *domainRuntime.ArrowRuntime) error {
	if current == nil || current.Ref == "" {
		return fmt.Errorf("begin stop: %w", asynxModels.ErrValidation)
	}
	if current.Execution != nil && current.Execution.Method == domain.MethodStop {
		return fmt.Errorf("begin stop: %w", asynxModels.ErrValidation)
	}
	switch current.State { //nolint:exhaustive
	case domain.ArrowStateRunning, domain.ArrowStateDetached:
		return nil
	default:
	}
	return fmt.Errorf("begin stop: %w", asynxModels.ErrValidation)
}

func (c BeginStop) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	pid := 0
	if current != nil && current.Execution != nil {
		pid = current.Execution.PID
	}
	return domainRuntime.ArrowRuntime{
		Ref:   c.Namespace,
		State: domain.ArrowStateStopping,
		Execution: &domainRuntime.Execution{
			Method:    domain.MethodStop,
			Steps:     initialSteps(c.Steps),
			Variables: c.Variables,
			PID:       pid,
			WorkDir:   c.WorkDir,
		},
		LastReturn: preserveLastReturn(current),
	}
}
