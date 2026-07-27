// Package providers holds one implementation per git host. Everything that is
// true of a host and not of git — its search dialect, its raw-file URL shape,
// the redirect its release permalink answers with — lives here, one file per
// host, and nothing outside the provider engine imports it.
package providers

import (
	"context"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// Provider answers for one git host.
type Provider interface {
	// Host is the domain this provider answers for, e.g. "github.com".
	Host() string

	// CanSearch reports whether this host exposes a repository search Quiver
	// can query. A host that cannot is still a provider: it serves manifests,
	// it just never contributes a candidate.
	CanSearch() bool

	// Search returns the candidates matching req. A rate-limited or
	// unauthorised host returns *RateLimitedError or *UnauthorizedError so the
	// caller can tell those apart from an empty result set. A host that cannot
	// search returns ErrSearchUnsupported.
	Search(
		ctx context.Context,
		req SearchRequest,
	) ([]Candidate, error)

	// LatestRelease is the ref the host's latest stable release carries. It
	// returns ErrNoLatestRelease when the host publishes none, which is a miss
	// and not a failure.
	LatestRelease(
		ctx context.Context,
		ns domain.Namespace,
	) (string, error)

	// RawFileURL is where file lives at ref inside the repository ns names.
	RawFileURL(
		ns domain.Namespace,
		ref string,
		file string,
	) (string, error)

	// DefaultBranches are the refs to try, in order, for a namespace that
	// carries none.
	DefaultBranches() []string
}

// DoFunc issues one HTTP request. It exists so tests can supply canned
// responses without a socket; production wiring leaves it nil and gets fns.
type DoFunc func(
	ctx context.Context,
	req fns.Request,
) (fns.Response, error)

// Config describes one host: which one it is, and where to send each question
// it can answer. Do and Now default to the process HTTP client and clock when
// left nil.
//
// Kind names the host itself. No implementation reads it — it is what picks the
// implementation, so it is read once, by the constructor.
type Config struct {
	Host             string
	Kind             string
	RawURL           string
	DefaultBranches  []string
	LatestReleaseURL string
	SearchURL        string
	Timeout          time.Duration
	Do               DoFunc
	Now              func() time.Time
}
