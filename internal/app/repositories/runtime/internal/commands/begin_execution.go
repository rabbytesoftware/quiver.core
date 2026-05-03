package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
)

type BeginExecution struct {
	Namespace   domain.Namespace
	Method      string
	AvailableIn []domain.ArrowState
	Steps       []domainStep.Step
	Variables   map[string]string
	WorkDir     string
}

func (c BeginExecution) AggregateID() string {
	return c.Namespace.String()
}

func (c BeginExecution) EventName() string {
	return "runtime.begun." + c.Namespace.String()
}

func (c BeginExecution) ShouldSnapshot() bool {
	return true
}

func (c BeginExecution) Validate(current *domainRuntime.ArrowRuntime) error {
	absent := current == nil || current.Ref == ""

	if absent { //nolint:nestif
		if c.AvailableIn == nil {
			return nil
		}
		for _, s := range c.AvailableIn {
			if s == domain.ArrowStateAbsent {
				return nil
			}
		}
		return fmt.Errorf("begin execution: %w", asynxModels.ErrValidation)
	}

	if current.Execution != nil {
		if c.Method != domain.MethodStop || current.Execution.Method == domain.MethodStop {
			return fmt.Errorf("begin execution: %w", asynxModels.ErrValidation)
		}
	}

	if c.AvailableIn == nil {
		return nil
	}

	for _, s := range c.AvailableIn {
		if s == current.State {
			return nil
		}
	}

	return fmt.Errorf("begin execution: %w", asynxModels.ErrValidation)
}

func (c BeginExecution) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	newState := stateForMethod(c.Method)
	pid := 0
	if c.Method == domain.MethodStop && current != nil && current.Execution != nil {
		pid = current.Execution.PID
	}

	return domainRuntime.ArrowRuntime{
		Ref:   c.Namespace,
		State: newState,
		Execution: &domainRuntime.Execution{
			Method:    c.Method,
			Steps:     initialSteps(c.Steps),
			Variables: c.Variables,
			PID:       pid,
			WorkDir:   c.WorkDir,
		},
		LastReturn: preserveLastReturn(current),
	}
}

func stateForMethod(method string) domain.ArrowState {
	switch method {
	case domain.MethodInstall:
		return domain.ArrowStateInstalling
	case domain.MethodUninstall:
		return domain.ArrowStateUninstalling
	case domain.MethodUpdate:
		return domain.ArrowStateUpdating
	case domain.MethodStop:
		return domain.ArrowStateStopping
	default:
		return domain.ArrowStateRunning
	}
}

func initialSteps(steps []domainStep.Step) []domainRuntime.StepProgress {
	result := make([]domainRuntime.StepProgress, len(steps))
	for i, s := range steps {
		result[i] = domainRuntime.StepProgress{
			Index:  i,
			Status: domainRuntime.StepStatusPending,
			Step:   s,
		}
	}
	return result
}

func preserveLastReturn(current *domainRuntime.ArrowRuntime) *domainRuntime.Return {
	if current == nil {
		return nil
	}
	return current.LastReturn
}
