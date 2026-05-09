package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// RecoverInterrupted resets an arrow stuck in a transient state to a safe stable
// state after a crash or unexpected shutdown.
//
// Recovery mapping:
//
//	installing   → absent  (install didn't finish; binaries not guaranteed)
//	uninstalling → absent  (partial uninstall = treat as not installed)
//	updating     → absent  (partial update = unsafe; needs reinstall)
//	running      → ready   (dead-PID path; alive-PID path uses RecordDetached instead)
//	stopping     → ready   (stop interrupted; process is gone after crash)
//	draining     → ready   (drain cascade interrupted; deps already cleaned up by their own recovery)
//
// ArrowStateDetached is intentionally excluded: arrows already in detached are
// stable and idempotent (Validate rejects them). Their process survived the crash
// and is handled separately via RecordDetached on startup.
type RecoverInterrupted struct {
	Namespace domain.Namespace
}

func (c RecoverInterrupted) AggregateID() string {
	return c.Namespace.String()
}

func (c RecoverInterrupted) EventName() string {
	return "runtime.recovered." + c.Namespace.String()
}

func (c RecoverInterrupted) ShouldSnapshot() bool {
	return true
}

func (c RecoverInterrupted) Validate(current *domainRuntime.ArrowRuntime) error {
	if current == nil || current.Ref == "" {
		return fmt.Errorf("recover interrupted: %w", asynxModels.ErrValidation)
	}
	switch current.State {
	case domain.ArrowStateInstalling,
		domain.ArrowStateUninstalling,
		domain.ArrowStateUpdating,
		domain.ArrowStateRunning,
		domain.ArrowStateStopping,
		domain.ArrowStateDraining:
		return nil
	case domain.ArrowStateAbsent,
		domain.ArrowStateReady,
		domain.ArrowStateDetached,
		domain.ArrowStateRemoved,
		domain.ArrowStateOutdated:
	}
	return fmt.Errorf("recover interrupted: %w", asynxModels.ErrValidation)
}

func (c RecoverInterrupted) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:        c.Namespace,
		State:      stableStateFor(current.State),
		Execution:  nil,
		LastReturn: current.LastReturn,
	}
}

func stableStateFor(s domain.ArrowState) domain.ArrowState {
	switch s {
	case domain.ArrowStateInstalling,
		domain.ArrowStateUninstalling,
		domain.ArrowStateUpdating:
		return domain.ArrowStateAbsent
	case domain.ArrowStateRunning,
		domain.ArrowStateStopping,
		domain.ArrowStateDraining,
		domain.ArrowStateAbsent,
		domain.ArrowStateReady,
		domain.ArrowStateDetached,
		domain.ArrowStateRemoved,
		domain.ArrowStateOutdated:
		return domain.ArrowStateReady
	}
	return domain.ArrowStateReady
}
