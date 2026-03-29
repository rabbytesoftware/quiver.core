package resolver

import "errors"

// ErrUnsupportedPlatform is returned when a namespace domain is not one of
// the supported git hosting platforms.
var ErrUnsupportedPlatform = errors.New("resolver: unsupported git platform")

// ErrNotFound is returned when the manifest file does not exist in the
// remote repository.
var ErrNotFound = errors.New("resolver: manifest not found")

// ErrFetchFailed is returned when the remote git repository cannot be cloned
// or the file cannot be read from the in-memory worktree.
var ErrFetchFailed = errors.New("resolver: git fetch failed")
