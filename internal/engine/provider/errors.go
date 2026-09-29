package provider

import (
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider/providers"
)

type (
	// RateLimitedError says a host refused the search because the caller is out
	// of budget.
	RateLimitedError = providers.RateLimitedError

	// UnauthorizedError says a host rejected the caller's credentials.
	UnauthorizedError = providers.UnauthorizedError
)

// ErrNoLatestRelease reports that a host cannot name a latest stable release
// for a namespace. It is a miss, not a failure.
var ErrNoLatestRelease = providers.ErrNoLatestRelease

// ErrSearchUnsupported reports that a host exposes no repository search.
var ErrSearchUnsupported = providers.ErrSearchUnsupported

// ErrNoRawURL reports that a host serves no raw files over HTTP.
var ErrNoRawURL = providers.ErrNoRawURL
