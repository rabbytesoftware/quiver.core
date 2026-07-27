// Package provider answers the questions only a git host can answer: how to
// search it, where it serves a raw file, which refs it defaults to, and which
// ref its latest release carries. It knows nothing about arrows — it never
// fetches or parses a manifest.
//
// This package is the whole vocabulary a caller needs: the per-host
// implementations live in the providers subpackage and are never named from
// outside.
package provider

import (
	"context"
	"fmt"
	"slices"

	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider/providers"
)

type (
	// Candidate is one repository carrying a discovery marker. Whether it
	// really is an arrow is decided later, by fetching and parsing its
	// manifest.
	Candidate = providers.Candidate

	// SearchRequest is one query against one host.
	SearchRequest = providers.SearchRequest

	// Config describes one provider.
	Config = providers.Config

	// DoFunc issues one HTTP request, so tests can supply canned responses
	// without a socket.
	DoFunc = providers.DoFunc
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

// New builds the provider for the host cfg names. cfg.Kind decides how that
// host's answers are read, so a kind with no implementation behind it is an
// error rather than a guess.
func New(
	cfg Config,
) (Provider, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("provider: no host configured")
	}

	switch cfg.Kind {
	case metadata.KindGitHub:
		return providers.NewGitHub(cfg), nil
	case metadata.KindGitLab:
		return providers.NewGitLab(cfg), nil
	case metadata.KindBitbucket:
		return providers.NewBitbucket(cfg), nil
	}
	return nil, fmt.Errorf("provider %s: unknown kind %q", cfg.Host, cfg.Kind)
}

// FromPlatforms builds one provider per platform, ordered by host so the
// provider set is stable.
//
// Every platform is a provider: a host serves manifests whether or not it also
// answers queries, and dropping the ones that cannot search would take their
// raw-file URLs with them. Search is a capability the caller filters on.
func FromPlatforms(
	platforms metadata.Platforms,
	base Config,
) ([]Provider, error) {
	hosts := make([]string, 0, len(platforms))
	for host := range platforms {
		hosts = append(hosts, host)
	}
	slices.Sort(hosts)

	built := make([]Provider, 0, len(hosts))
	for _, host := range hosts {
		p, err := New(configFor(base, host, platforms[host]))
		if err != nil {
			return nil, fmt.Errorf("provider: build %s: %w", host, err)
		}
		built = append(built, p)
	}
	return built, nil
}

// configFor fills the platform's own endpoints into the caller's base config,
// which carries what is true of every provider: its timeouts and its transport.
func configFor(
	base Config,
	host string,
	platform metadata.Platform,
) Config {
	cfg := base
	cfg.Host = host
	cfg.Kind = platform.Kind
	cfg.RawURL = platform.RawURL
	cfg.DefaultBranches = platform.DefaultBranches
	cfg.LatestReleaseURL = platform.LatestReleaseURL
	cfg.SearchURL = platform.SearchURL
	return cfg
}
