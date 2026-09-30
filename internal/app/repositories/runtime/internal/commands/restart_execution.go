package commands

import (
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

// RestartExecution replaces the running execution's steps with fresh, pending
// ones and keeps everything else about the run: its id, method, variables and
// workdir. Its event is named as a step advance on purpose, so the step
// progress subscribers republish the reset snapshot without a topic of their own.
type RestartExecution struct {
	Namespace   domain.Namespace
	ExecutionID string
	Steps       []domainStep.Step
}

func (c RestartExecution) AggregateID() string {
	return c.Namespace.String()
}

func (c RestartExecution) EventName() string {
	return "runtime.step_advanced." + c.Namespace.String()
}

func (c RestartExecution) ShouldSnapshot() bool {
	return true
}

func (c RestartExecution) Validate(current *domainRuntime.ArrowRuntime) error {
	return requireCurrentExecution("restart execution", current, c.ExecutionID)
}

func (c RestartExecution) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:   current.Ref,
		State: current.State,
		Execution: &domainRuntime.Execution{
			ID:        current.Execution.ID,
			Method:    current.Execution.Method,
			Steps:     initialSteps(c.Steps),
			Variables: current.Execution.Variables,
			WorkDir:   current.Execution.WorkDir,
		},
		LastReturn:     current.LastReturn,
		PendingDepSync: current.PendingDepSync,
	}
}
