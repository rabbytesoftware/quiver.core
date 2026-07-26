package resolvers

import "errors"

// ErrNotFound is returned when the manifest file does not exist in the
// remote repository.
var ErrNotFound = errors.New("resolver: manifest not found")

// ErrFetchFailed is returned when the remote git repository cannot be cloned,
// HTTP request fails, or the file cannot be read from the in-memory worktree.
var ErrFetchFailed = errors.New("resolver: fetch failed")

// ErrNoLatestRelease is returned when a platform cannot name a latest stable
// release for a namespace. It is a miss, not a failure: the caller falls
// through to the next step of the ref-resolution chain.
var ErrNoLatestRelease = errors.New("resolver: no latest release")
