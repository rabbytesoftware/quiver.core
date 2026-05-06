package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
)

// BeginExecution starts a custom or built-in execute method from Ready state.
// For install/uninstall/stop/update use the dedicated Begin* commands.
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
	if current == nil || current.Ref == "" {
		return fmt.Errorf("begin execution: %w", asynxModels.ErrValidation)
	}
	if current.Execution != nil {
		return fmt.Errorf("begin execution: %w", asynxModels.ErrValidation)
	}
	if len(c.AvailableIn) == 0 {
		if current.State != domain.ArrowStateReady {
			return fmt.Errorf("begin execution: %w", asynxModels.ErrValidation)
		}
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
	return domainRuntime.ArrowRuntime{
		Ref:   c.Namespace,
		State: domain.ArrowStateRunning,
		Execution: &domainRuntime.Execution{
			Method:    c.Method,
			Steps:     initialSteps(c.Steps),
			Variables: c.Variables,
			WorkDir:   c.WorkDir,
		},
		LastReturn: preserveLastReturn(current),
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
