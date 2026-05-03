package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

type AdvanceStep struct {
	Namespace domain.Namespace
	StepIndex int
	ToStatus  domainRuntime.StepStatus
	Error     *string
}

func (c AdvanceStep) AggregateID() string {
	return c.Namespace.String()
}

func (c AdvanceStep) EventName() string {
	return "runtime.step_advanced." + c.Namespace.String()
}

func (c AdvanceStep) ShouldSnapshot() bool {
	return false
}

func (c AdvanceStep) Validate(current *domainRuntime.ArrowRuntime) error {
	if current == nil || current.Ref == "" {
		return fmt.Errorf("advance step: %w", asynxModels.ErrValidation)
	}
	if current.Execution == nil {
		return fmt.Errorf("advance step: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c AdvanceStep) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	steps := make([]domainRuntime.StepProgress, len(current.Execution.Steps))
	copy(steps, current.Execution.Steps)
	steps[c.StepIndex].Status = c.ToStatus
	steps[c.StepIndex].Error = c.Error

	updatedRun := &domainRuntime.Execution{
		Method:    current.Execution.Method,
		Steps:     steps,
		Variables: current.Execution.Variables,
		PID:       current.Execution.PID,
	}

	return domainRuntime.ArrowRuntime{
		Ref:        current.Ref,
		State:      current.State,
		Execution:  updatedRun,
		LastReturn: current.LastReturn,
	}
}
