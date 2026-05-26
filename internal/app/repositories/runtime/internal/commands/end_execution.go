package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

type EndExecution struct {
	Namespace domain.Namespace
	Outcome   domainRuntime.ExecutionOutcome
}

func (c EndExecution) AggregateID() string {
	return c.Namespace.String()
}

func (c EndExecution) EventName() string {
	return "runtime.ended." + c.Namespace.String()
}

func (c EndExecution) ShouldSnapshot() bool {
	return true
}

func (c EndExecution) Validate(current *domainRuntime.ArrowRuntime) error {
	if current == nil || current.Ref == "" {
		return fmt.Errorf("end execution: %w", asynxModels.ErrValidation)
	}
	if current.Execution == nil {
		return fmt.Errorf("end execution: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c EndExecution) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	exec := current.Execution

	ret := domainRuntime.Return{
		Method:    exec.Method,
		Outcome:   c.Outcome,
		Steps:     exec.Steps,
		Variables: exec.Variables,
	}

	newState := stateAfterEnd(exec.Method, c.Outcome)

	return domainRuntime.ArrowRuntime{
		Ref:        c.Namespace,
		State:      newState,
		Execution:  nil,
		LastReturn: &ret,
	}
}

func stateAfterEnd(method string, outcome domainRuntime.ExecutionOutcome) domain.ArrowState {
	switch method {
	case domain.MethodInstall:
		if outcome == domainRuntime.ExecutionOutcomeSuccess {
			return domain.ArrowStateReady
		}
		return domain.ArrowStateAbsent
	case domain.MethodUninstall:
		if outcome == domainRuntime.ExecutionOutcomeSuccess {
			return domain.ArrowStateAbsent
		}
		return domain.ArrowStateReady
	default:
		return domain.ArrowStateReady
	}
}
