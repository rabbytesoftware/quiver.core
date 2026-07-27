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
	"fmt"
	"slices"

	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider/providers"
)

type (
	// Provider answers for one git host. Aliased rather than redeclared: an
	// identical copy here would compile, then drift in its documentation.
	Provider = providers.Provider

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
