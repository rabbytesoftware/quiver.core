package providers

import "errors"

// ErrSearchUnsupported reports that a host exposes no repository search. It is
// what a provider answers when it is asked anyway; callers that can choose ask
// CanSearch first.
var ErrSearchUnsupported = errors.New("provider: host exposes no search")

// ErrNoRawURL reports that a host serves no raw files over HTTP, so a manifest
// there can only be reached by cloning.
var ErrNoRawURL = errors.New("provider: host serves no raw files")

var ErrNoBlobURL = errors.New("provider: host renders no files")

// ErrNoRepoMetadata reports that a host offers no repository metadata, or that
// asking for it failed. The caller degrades to what the repository page says.
var ErrNoRepoMetadata = errors.New("provider: no repository metadata")

var ErrUnexpectedPage = errors.New("provider: page did not match the expected shape")
