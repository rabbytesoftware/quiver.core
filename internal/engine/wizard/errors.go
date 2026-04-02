package wizard

import (
	"errors"
	"fmt"

	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
)

var ErrUnknownStepType = errors.New("wizard: unknown step type")
var ErrExecutionExists = errors.New("wizard: execution already in progress for namespace")

// StepError wraps a handler error with the step index and definition.
// Inspect Cause with errors.Is/As for handler-specific errors:
//   - run.ErrNonZeroExit
//   - signal.ErrNoProcess, signal.ErrInvalidSignal
type StepError struct {
	Index int
	Step  step.Step
	Cause error
}

func (e *StepError) Error() string {
	return fmt.Sprintf("step %d failed: %v", e.Index, e.Cause)
}

func (e *StepError) Unwrap() error {
	return e.Cause
}
