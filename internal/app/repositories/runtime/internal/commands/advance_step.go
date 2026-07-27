package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

type AdvanceStep struct {
	Namespace   domain.Namespace
	ExecutionID string
	StepIndex   int
	ToStatus    domainRuntime.StepStatus
	Error       *string
}

func (c AdvanceStep) AggregateID() string {
	return c.Namespace.String()
}

func (c AdvanceStep) EventName() string {
	return "runtime.step_advanced." + c.Namespace.String()
}

// ShouldSnapshot is true even though this is a high-frequency command. Under
// asynx v0.8 a snapshot is a single upserted row, not an appended one, so
// snapshotting every step advance costs O(1) per write instead of making
// every future read slower.
func (c AdvanceStep) ShouldSnapshot() bool {
	return true
}

func (c AdvanceStep) Validate(current *domainRuntime.ArrowRuntime) error {
	if err := requireCurrentExecution("advance step", current, c.ExecutionID); err != nil {
		return err
	}
	if c.StepIndex < 0 || c.StepIndex >= len(current.Execution.Steps) {
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
		ID:        current.Execution.ID,
		Method:    current.Execution.Method,
		Steps:     steps,
		Variables: current.Execution.Variables,
		PID:       current.Execution.PID,
		WorkDir:   current.Execution.WorkDir,
	}

	return domainRuntime.ArrowRuntime{
		Ref:        current.Ref,
		State:      current.State,
		Execution:  updatedRun,
		LastReturn: current.LastReturn,
	}
}
