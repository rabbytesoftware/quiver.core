package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
)

type BeginUninstall struct {
	Namespace domain.Namespace
	Steps     []domainStep.Step
	Variables map[string]string
	WorkDir   string
}

func (c BeginUninstall) AggregateID() string  { return c.Namespace.String() }
func (c BeginUninstall) EventName() string    { return "runtime.begun." + c.Namespace.String() }
func (c BeginUninstall) ShouldSnapshot() bool { return true }

func (c BeginUninstall) Validate(current *domainRuntime.ArrowRuntime) error {
	if current == nil || current.Ref == "" || current.State != domain.ArrowStateReady {
		return fmt.Errorf("begin uninstall: %w", asynxModels.ErrValidation)
	}
	if current.Execution != nil {
		return fmt.Errorf("begin uninstall: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c BeginUninstall) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:   c.Namespace,
		State: domain.ArrowStateUninstalling,
		Execution: &domainRuntime.Execution{
			Method:    domain.MethodUninstall,
			Steps:     initialSteps(c.Steps),
			Variables: c.Variables,
			WorkDir:   c.WorkDir,
		},
		LastReturn: preserveLastReturn(current),
	}
}
