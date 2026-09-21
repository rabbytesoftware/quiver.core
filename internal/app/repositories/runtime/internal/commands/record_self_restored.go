package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// RecordSelfRestored confirms a still-alive arrow as quiver.core's own
// deliberate self-update restart rather than an unexplained crash, and lands
// it in Running with its Execution tracking intact. Unlike RecordDetached,
// this never drops Execution — the process was never actually unsupervised,
// quiver.core just briefly couldn't tell the two cases apart while it was
// restarting itself.
//
// The accepted current state is Running or Detached: recoverRunning's one
// real caller always observes Running (RecoverTransients only calls it from
// its Running branch, so this command replaces RecordDetached outright rather
// than following it), while Detached covers a namespace already marked
// detached moments earlier, before quiver.core could tell the two cases apart.
type RecordSelfRestored struct {
	Namespace domain.Namespace
	Execution *domainRuntime.Execution
}

func (c RecordSelfRestored) AggregateID() string {
	return c.Namespace.String()
}

func (c RecordSelfRestored) EventName() string {
	return "runtime.self_restored." + c.Namespace.String()
}

func (c RecordSelfRestored) ShouldSnapshot() bool {
	return true
}

func (c RecordSelfRestored) Validate(current *domainRuntime.ArrowRuntime) error {
	if current == nil || current.Ref == "" {
		return fmt.Errorf("record self restored: %w", asynxModels.ErrValidation)
	}
	if current.State != domain.ArrowStateDetached && current.State != domain.ArrowStateRunning {
		return fmt.Errorf("record self restored: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c RecordSelfRestored) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	return domainRuntime.ArrowRuntime{
		Ref:        current.Ref,
		State:      domain.ArrowStateRunning,
		Execution:  c.Execution,
		LastReturn: current.LastReturn,
	}
}
