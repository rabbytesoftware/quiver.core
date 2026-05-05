package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
)

type BeginUpdate struct {
	Namespace domain.Namespace
	Steps     []domainStep.Step
	Variables map[string]string
	WorkDir   string
}

func (c BeginUpdate) AggregateID() string  { return c.Namespace.String() }
func (c BeginUpdate) EventName() string    { return "runtime.begun." + c.Namespace.String() }
func (c BeginUpdate) ShouldSnapshot() bool { return true }

func (c BeginUpdate) Validate(current *domainRuntime.ArrowRuntime) error {
	if current == nil || current.Ref == "" || current.State != domain.ArrowStateOutdated {
		return fmt.Errorf("begin update: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c BeginUpdate) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:   c.Namespace,
		State: domain.ArrowStateUpdating,
		Execution: &domainRuntime.Execution{
			Method:    domain.MethodUpdate,
			Steps:     initialSteps(c.Steps),
			Variables: c.Variables,
			WorkDir:   c.WorkDir,
		},
		LastReturn:     preserveLastReturn(current),
		PendingDepSync: nil,
	}
}
