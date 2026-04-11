package errors

import "errors"

var (
	ErrNotFound         = errors.New("not found")
	ErrAlreadyExists    = errors.New("already exists")
	ErrAlreadyRemoved   = errors.New("already removed")
	ErrStateViolation   = errors.New("state violation")
	ErrFetchFailed      = errors.New("fetch failed")
	ErrInvalidNamespace = errors.New("invalid namespace")
	ErrDependentsExist  = errors.New("other arrows depend on this arrow")
	ErrInvalidManifest  = errors.New("invalid manifest")
)
