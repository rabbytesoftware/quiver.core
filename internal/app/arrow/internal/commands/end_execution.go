package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

type EndExecution struct {
	Namespace domain.Namespace
	Outcome   domainRuntime.ExecutionOutcome
}

func (c EndExecution) AggregateID() string {
	return c.Namespace.String()
}

func (c EndExecution) EventName() string {
	return "runtime.ended"
}

func (c EndExecution) ShouldSnapshot() bool {
	return true
}

func (c EndExecution) Validate(current *domainRuntime.ArrowRuntime) error {
	if current == nil || current.Namespace == "" {
		return fmt.Errorf("end execution: %w", asynxModels.ErrValidation)
	}
	if current.ActiveRun == nil {
		return fmt.Errorf("end execution: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c EndExecution) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	activeRun := current.ActiveRun

	ret := domainRuntime.Return{
		Method:    activeRun.Method,
		Outcome:   c.Outcome,
		Steps:     activeRun.Steps,
		Variables: activeRun.Variables,
	}

	newState := stateAfterEnd(activeRun.Method, c.Outcome)

	return domainRuntime.ArrowRuntime{
		Namespace:  c.Namespace,
		State:      newState,
		ActiveRun:  nil,
		LastReturn: &ret,
	}
}

func stateAfterEnd(method string, outcome domainRuntime.ExecutionOutcome) domain.ArrowState {
	switch method {
	case "_install":
		if outcome == domainRuntime.ExecutionOutcomeSuccess {
			return domain.ArrowStateReady
		}
		return domain.ArrowStateAbsent
	case "_uninstall":
		if outcome == domainRuntime.ExecutionOutcomeSuccess {
			return domain.ArrowStateAbsent
		}
		return domain.ArrowStateReady
	default:
		return domain.ArrowStateReady
	}
}
