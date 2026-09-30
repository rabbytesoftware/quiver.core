package resolvers

import (
	"errors"
	"fmt"
)

// ErrNotFound is returned when the manifest file does not exist in the
// remote repository.
var ErrNotFound = errors.New("resolver: manifest not found")

var ErrManifestNotFound = fmt.Errorf("%w: no manifest file", ErrNotFound)

var ErrAbsentAtRef = fmt.Errorf("%w: absent at the pinned ref", ErrNotFound)

// ErrFetchFailed is returned when the remote git repository cannot be cloned,
// HTTP request fails, or the file cannot be read from the in-memory worktree.
var ErrFetchFailed = errors.New("resolver: fetch failed")
