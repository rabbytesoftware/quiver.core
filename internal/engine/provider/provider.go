// Package provider turns a git host's repository-search API into a stream of
// arrow candidates. It knows HTTP and search dialects; it knows nothing about
// arrows and never fetches a manifest.
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

// Provider searches one git host for arrow candidates.
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

// New builds the provider implementing cfg.SearchKind.
func New(
	cfg Config,
) (Provider, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("provider: no host configured")
	}
	if cfg.SearchURL == "" {
		return nil, fmt.Errorf("provider %s: no search url configured", cfg.Host)
	}

	switch cfg.SearchKind {
	case metadata.SearchKindGitHub:
		return providers.NewGitHub(cfg), nil
	case metadata.SearchKindGitLab:
		return providers.NewGitLab(cfg), nil
	}
	return nil, fmt.Errorf("provider %s: unknown search kind %q", cfg.Host, cfg.SearchKind)
}

// FromPlatforms builds one provider per platform that declares a search URL,
// ordered by host so the provider set is stable. A platform without a search
// URL has no usable search API and is excluded on purpose, not by accident.
func FromPlatforms(
	platforms metadata.Platforms,
	base Config,
) ([]Provider, error) {
	hosts := make([]string, 0, len(platforms))
	for host, platform := range platforms {
		if platform.SearchURL == "" {
			continue
		}
		hosts = append(hosts, host)
	}
	slices.Sort(hosts)

	built := make([]Provider, 0, len(hosts))
	for _, host := range hosts {
		cfg := base
		cfg.Host = host
		cfg.SearchURL = platforms[host].SearchURL
		cfg.SearchKind = platforms[host].SearchKind

		p, err := New(cfg)
		if err != nil {
			return nil, fmt.Errorf("provider: build %s: %w", host, err)
		}
		built = append(built, p)
	}
	return built, nil
}
