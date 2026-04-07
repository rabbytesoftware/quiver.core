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
}

func (c BeginExecution) AggregateID() string {
	return c.Namespace.String()
}

func (c BeginExecution) EventName() string {
	return "runtime.begun"
}

func (c BeginExecution) ShouldSnapshot() bool {
	return false
}

func (c BeginExecution) Validate(current *domainRuntime.ArrowRuntime) error {
	absent := current == nil || current.Namespace == ""

	if absent {
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

	if current.ActiveRun != nil {
		return fmt.Errorf("begin execution: %w", asynxModels.ErrValidation)
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

	return domainRuntime.ArrowRuntime{
		Namespace:  c.Namespace,
		State:      newState,
		ActiveRun:  &domainRuntime.RunRecord{Method: c.Method, Steps: initialSteps(c.Steps), Variables: c.Variables},
		LastReturn: preserveLastReturn(current),
	}
}

func stateForMethod(method string) domain.ArrowState {
	switch method {
	case "_install":
		return domain.ArrowStateInstalling
	case "_uninstall":
		return domain.ArrowStateUninstalling
	case "_stop":
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
