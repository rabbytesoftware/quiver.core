package errors

import "errors"

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
)
