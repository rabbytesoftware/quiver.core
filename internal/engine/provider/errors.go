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
