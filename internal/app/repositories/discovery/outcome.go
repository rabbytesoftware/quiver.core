package discovery

import (
	"errors"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
)

// Reasons a provider contributed nothing. A rate limit is temporary and carries
// how long to wait; an unauthorized host needs the user to fix a token.
const (
	ReasonRateLimited  = "rate_limited"
	ReasonUnauthorized = "unauthorized"
	ReasonError        = "error"
)

// Outcome summarises one discovery pass. Found counts the deduplicated
// candidates considered, Verified those that proved to be arrows, and Skipped
// those that did not.
type Outcome struct {
	Found     int
	Verified  int
	Skipped   int
	Providers []ProviderOutcome
}

// ProviderOutcome is what one host contributed. Every provider failing is a
// populated outcome, not an error: the caller must be able to say which host
// failed and why rather than being shown a bare failure.
type ProviderOutcome struct {
	Host       string
	OK         bool
	Returned   int
	Reason     string
	RetryAfter time.Duration
}

func outcomeOf(
	host string,
	candidates []provider.Candidate,
	err error,
) ProviderOutcome {
	if err == nil {
		return ProviderOutcome{Host: host, OK: true, Returned: len(candidates)}
	}

	outcome := ProviderOutcome{Host: host, Reason: ReasonError}

	var limited *provider.RateLimitedError
	if errors.As(err, &limited) {
		outcome.Reason = ReasonRateLimited
		outcome.RetryAfter = limited.RetryAfter
		return outcome
	}

	var unauthorized *provider.UnauthorizedError
	if errors.As(err, &unauthorized) {
		outcome.Reason = ReasonUnauthorized
	}
	return outcome
}
