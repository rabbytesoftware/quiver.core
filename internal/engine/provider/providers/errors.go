package providers

import "errors"

// ErrNoLatestRelease reports that a host cannot name a latest stable release
// for a namespace. It is a miss, not a failure: the caller falls through to the
// next step of the ref-resolution chain.
var ErrNoLatestRelease = errors.New("provider: no latest release")

// ErrSearchUnsupported reports that a host exposes no repository search. It is
// what a provider answers when it is asked anyway; callers that can choose ask
// CanSearch first.
var ErrSearchUnsupported = errors.New("provider: host exposes no search")

// ErrNoRawURL reports that a host serves no raw files over HTTP, so a manifest
// there can only be reached by cloning.
var ErrNoRawURL = errors.New("provider: host serves no raw files")
