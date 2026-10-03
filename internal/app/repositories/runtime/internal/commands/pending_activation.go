package commands

import (
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

// RecordPendingActivation stages a binary a finished method produced, to be
// applied by a daemon restart. A later stage replaces an earlier one that has
// not started activating, since the later fetch was verified on its own.
type RecordPendingActivation struct {
	Namespace domain.Namespace
	Pending   domainRuntime.PendingActivation
}

func (c RecordPendingActivation) AggregateID() string  { return c.Namespace.String() }
func (c RecordPendingActivation) ShouldSnapshot() bool { return true }

func (c RecordPendingActivation) EventName() string {
	return "runtime.activation_staged." + c.Namespace.String()
}

func (c RecordPendingActivation) Validate(current *domainRuntime.ArrowRuntime) error {
	if current == nil || current.Ref == "" || current.Execution != nil {
		return fmt.Errorf("record pending activation: %w", asynxModels.ErrValidation)
	}
	if c.Pending.Version == "" || c.Pending.Path == "" {
		return fmt.Errorf("record pending activation: version and path are required: %w", asynxModels.ErrValidation)
	}
	if current.PendingActivation != nil && current.PendingActivation.Activating {
		return fmt.Errorf("record pending activation: the earlier one is activating: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c RecordPendingActivation) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	next := *current
	pending := c.Pending
	next.PendingActivation = &pending
	return next
}

// MarkActivating records that the daemon is about to hand over to the staged
// binary.
type MarkActivating struct {
	Namespace domain.Namespace
}

func (c MarkActivating) AggregateID() string  { return c.Namespace.String() }
func (c MarkActivating) ShouldSnapshot() bool { return true }

func (c MarkActivating) EventName() string {
	return "runtime.activation_marked." + c.Namespace.String()
}

func (c MarkActivating) Validate(current *domainRuntime.ArrowRuntime) error {
	if current == nil || current.Ref == "" || current.PendingActivation == nil || current.PendingActivation.Activating {
		return fmt.Errorf("mark activating: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c MarkActivating) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	next := *current
	marked := *current.PendingActivation
	marked.Activating = true
	next.PendingActivation = &marked
	return next
}

// ClearPendingActivation forgets the staged binary: it was applied, discarded
// or its handover failed.
type ClearPendingActivation struct {
	Namespace domain.Namespace
}

func (c ClearPendingActivation) AggregateID() string  { return c.Namespace.String() }
func (c ClearPendingActivation) ShouldSnapshot() bool { return true }

func (c ClearPendingActivation) EventName() string {
	return "runtime.activation_cleared." + c.Namespace.String()
}

func (c ClearPendingActivation) Validate(current *domainRuntime.ArrowRuntime) error {
	if current == nil || current.Ref == "" || current.PendingActivation == nil {
		return fmt.Errorf("clear pending activation: %w", asynxModels.ErrValidation)
	}
	return nil
}

func (c ClearPendingActivation) EmitEvent(current *domainRuntime.ArrowRuntime) domainRuntime.ArrowRuntime {
	next := *current
	next.PendingActivation = nil
	return next
}

func preservePendingActivation(current *domainRuntime.ArrowRuntime) *domainRuntime.PendingActivation {
	if current == nil {
		return nil
	}
	return current.PendingActivation
}
