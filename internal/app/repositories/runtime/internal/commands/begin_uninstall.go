package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

type BeginUninstall struct {
	Namespace   domain.Namespace
	ExecutionID string
	Steps       []domainStep.Step
	Variables   map[string]string
	WorkDir     string
}

func (c BeginUninstall) AggregateID() string  { return c.Namespace.String() }
func (c BeginUninstall) EventName() string    { return "runtime.begun." + c.Namespace.String() }
func (c BeginUninstall) ShouldSnapshot() bool { return true }

// Validate accepts Outdated alongside Ready: an installed, idle arrow is
// uninstallable regardless of which reason ("a newer release exists" or "a
// dependency sync is pending") put it in Outdated — uninstalling it doesn't
// care why. Callers that already switch on ArrowStateReady/ArrowStateOutdated
// together (e.g. orphaned-dependency cleanup) depend on this to actually take
// effect rather than silently fail validation for the Outdated half of that
// switch.
func (c BeginUninstall) Validate(current *domainRuntime.ArrowRuntime) error {
	if current == nil || current.Ref == "" {
		return fmt.Errorf("begin uninstall: %w", asynxModels.ErrValidation)
	}
	if current.State != domain.ArrowStateReady && current.State != domain.ArrowStateOutdated {
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
			ID:        c.ExecutionID,
			Method:    domain.MethodUninstall,
			Steps:     initialSteps(c.Steps),
			Variables: c.Variables,
			WorkDir:   c.WorkDir,
		},
		LastReturn: preserveLastReturn(current),
	}
}
