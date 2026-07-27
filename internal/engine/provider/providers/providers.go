// Package providers holds one implementation per git host. Everything that is
// true of a host and not of git — its search dialect, its raw-file URL shape,
// the redirect its release permalink answers with — lives here, one file per
// host, and nothing outside the provider engine imports it.
package providers

import (
	"context"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
)

// Provider answers for one git host.
type Provider interface {
	// Host is the domain this provider answers for, e.g. "github.com".
	Host() string

	// Search returns the candidates matching req. A rate-limited or
	// unauthorised host returns *RateLimitedError or *UnauthorizedError so the
	// caller can tell those apart from an empty result set.
	Search(
		ctx context.Context,
		req SearchRequest,
	) ([]Candidate, error)
}

// DoFunc issues one HTTP request. It exists so tests can supply canned
// responses without a socket; production wiring leaves it nil and gets fns.
type DoFunc func(
	ctx context.Context,
	req fns.Request,
) (fns.Response, error)

// Config describes one host. Do and Now default to the process HTTP client and
// clock when left nil.
type Config struct {
	Host       string
	SearchURL  string
	SearchKind string
	Timeout    time.Duration
	Do         DoFunc
	Now        func() time.Time
}
