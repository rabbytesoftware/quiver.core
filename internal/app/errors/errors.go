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
	ErrInvalidPairingCode   = errors.New("invalid or expired pairing code")
	ErrUnauthorized         = errors.New("unauthorized")
	ErrChannelNotFound      = errors.New("channel not found")
	ErrReleaseUnresolved    = errors.New("release unresolved")
)

// ReleaseKind says why a release-bound variable could not be resolved.
type ReleaseKind string

const (
	ReleaseOffline             ReleaseKind = "offline"
	ReleaseRateLimited         ReleaseKind = "rate_limited"
	ReleaseNoRelease           ReleaseKind = "no_release"
	ReleaseNoAsset             ReleaseKind = "no_asset"
	ReleaseUnsupportedPlatform ReleaseKind = "unsupported_platform"
	ReleaseUnverifiable        ReleaseKind = "unverifiable"
)

// ReleaseError is a release that could not name what a run needs. It
// satisfies errors.Is(err, ErrReleaseUnresolved) and unwraps to the cause.
type ReleaseError struct {
	Kind ReleaseKind
	Err  error
}

// NewReleaseError builds a ReleaseError of kind caused by err.
func NewReleaseError(kind ReleaseKind, err error) *ReleaseError {
	return &ReleaseError{Kind: kind, Err: err}
}

func (e *ReleaseError) Error() string {
	return fmt.Sprintf("%s: %s: %v", ErrReleaseUnresolved, e.Kind, e.Err)
}

func (e *ReleaseError) Unwrap() []error { return []error{ErrReleaseUnresolved, e.Err} }

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
