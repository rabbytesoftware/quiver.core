package providers

import "errors"

// ErrSearchUnsupported reports that a host exposes no repository search. It is
// what a provider answers when it is asked anyway; callers that can choose ask
// CanSearch first.
var ErrSearchUnsupported = errors.New("provider: host exposes no search")

// ErrNoRawURL reports that a host serves no raw files over HTTP, so a manifest
// there can only be reached by cloning.
var ErrNoRawURL = errors.New("provider: host serves no raw files")
