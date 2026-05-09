package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

type BeginInstall struct {
	Namespace domain.Namespace
	Steps     []domainStep.Step
	Variables map[string]string
	WorkDir   string
}

func (c BeginInstall) AggregateID() string  { return c.Namespace.String() }
func (c BeginInstall) EventName() string    { return "runtime.begun." + c.Namespace.String() }
func (c BeginInstall) ShouldSnapshot() bool { return true }

func (c BeginInstall) Validate(current *domainRuntime.ArrowRuntime) error {
	if current == nil || current.Ref == "" {
		return nil
	}
	if current.Execution != nil {
		return fmt.Errorf("begin install: %w", asynxModels.ErrValidation)
	}
	switch current.State { //nolint:exhaustive
	case domain.ArrowStateAbsent, domain.ArrowStateRemoved:
		return nil
	default:
	}
	return fmt.Errorf("begin install: %w", asynxModels.ErrValidation)
}

func (c BeginInstall) EmitEvent(_ *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:   c.Namespace,
		State: domain.ArrowStateInstalling,
		Execution: &domainRuntime.Execution{
			Method:    domain.MethodInstall,
			Steps:     initialSteps(c.Steps),
			Variables: c.Variables,
			WorkDir:   c.WorkDir,
		},
	}
}
