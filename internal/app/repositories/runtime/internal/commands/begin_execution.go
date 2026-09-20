package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
	domainStep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
)

// BeginExecution starts a custom or built-in execute method from an installed,
// idle arrow — Ready, or the version-drift flavour of Outdated that means the
// same thing with a badge on it (see isVersionDriftOutdated).
// For install/uninstall/stop/update use the dedicated Begin* commands.
type BeginExecution struct {
	Namespace   domain.Namespace
	ExecutionID string
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
		if current.State != domain.ArrowStateReady && !isVersionDriftOutdated(current) {
			return fmt.Errorf("begin execution: %w", asynxModels.ErrValidation)
		}
		return nil
	}
	for _, s := range c.AvailableIn {
		if s == current.State {
			return nil
		}
		if s == domain.ArrowStateReady && isVersionDriftOutdated(current) {
			return nil
		}
	}
	return fmt.Errorf("begin execution: %w", asynxModels.ErrValidation)
}

// isVersionDriftOutdated separates the two facts Outdated is used to record,
// and reports only the one that does not stop an arrow being run: a passive
// version check found a newer release upstream. Nothing about the arrow itself
// changed — it is installed, idle and exactly as runnable as it was a moment
// earlier, so refusing to start it would make every arrow in the system
// unstartable the moment its upstream cut a release, until the user performed
// an update they never asked for. Applying an update must stay an explicit
// trigger, never a gate.
//
// A PendingDepSync is the other fact, and it is a genuine gate. MarkOutdated
// writes one to say the arrow's dependency graph changed and has not been
// re-synced; running against the wrong dependencies is the thing it exists to
// prevent, and runtimeUsecase.syncDeps — the only consumer of a PendingDepSync
// — requires the aggregate to still be Outdated when it runs, so starting from
// there would also strand the sync at Ready where nothing can pick it up
// again. ClearVersionOutdated already refuses the same aggregate for the same
// reason; this is that distinction applied on the way in.
//
// Outdated stands in for Ready and for nothing else. A method that declares
// available_in: [running] is still refused here, exactly as it was.
func isVersionDriftOutdated(current *domainRuntime.ArrowRuntime) bool {
	return current.State == domain.ArrowStateOutdated && current.PendingDepSync == nil
}

func (c BeginExecution) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:   c.Namespace,
		State: domain.ArrowStateRunning,
		Execution: &domainRuntime.Execution{
			ID:        c.ExecutionID,
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
