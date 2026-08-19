package errors

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound             = errors.New("not found")
	ErrAlreadyExists        = errors.New("already exists")
	ErrStateViolation       = errors.New("state violation")
	ErrMethodNotFound       = errors.New("method not found")
	ErrFetchFailed          = errors.New("fetch failed")
	ErrInvalidNamespace     = errors.New("invalid namespace")
	ErrDependentsExist      = errors.New("other arrows depend on this arrow")
	ErrInvalidManifest      = errors.New("invalid manifest")
	ErrPlatformNotSupported = errors.New("platform not supported")
	ErrMissingVariable      = errors.New("required variable not provided")
	ErrReservedVariable     = errors.New("variable is reserved by quiver and cannot be set")
	ErrExecutionSuperseded  = errors.New("execution superseded")
	ErrInvalidConfig        = errors.New("invalid config")
)

// StateViolationError describes an operation rejected because the arrow was in
// the wrong state. It satisfies errors.Is(err, ErrStateViolation), so existing
// sentinel checks and HTTP mapping keep working, while carrying the current
// state for a clearer message.
type StateViolationError struct {
	Op    string // the attempted operation (install, run, stop, uninstall, …)
	State string // the arrow's current state
}

// NewStateViolation builds a StateViolationError for op against state.
func NewStateViolation(op, state string) *StateViolationError {
	return &StateViolationError{Op: op, State: state}
}

func (e *StateViolationError) Error() string {
	if e.State == "" || e.State == "absent" {
		return fmt.Sprintf("cannot %s: arrow is not installed", e.Op)
	}
	return fmt.Sprintf("cannot %s: arrow is %s", e.Op, e.State)
}

func (e *StateViolationError) Unwrap() error { return ErrStateViolation }
