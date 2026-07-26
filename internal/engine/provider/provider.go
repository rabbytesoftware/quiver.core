// Package provider turns a git host's repository-search API into a stream of
// arrow candidates. It knows HTTP and search dialects; it knows nothing about
// arrows and never fetches a manifest.
package provider

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// Candidate is one repository carrying a discovery marker. Whether it really
// is an arrow is decided later, by fetching and parsing its manifest.
type Candidate struct {
	Namespace domain.Namespace
	Name      string
	// Description is whatever the host holds, which is not the manifest
	// description and may disagree with it.
	Description string
	Stars       int
	// Source is the host that answered, e.g. "github.com".
	Source string
	// DefaultBranch comes off the search response and is never guessed, so a
	// manifest fetch costs no extra request to discover it.
	DefaultBranch string
}

// SearchRequest is one query against one host. Topics are the discovery
// markers a repository must carry; hosts intersect them rather than union
// them. Limit caps the number of candidates returned.
type SearchRequest struct {
	Text   string
	Topics []string
	Limit  int
}

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

// DoFunc issues one HTTP request. It exists so tests can supply canned
// responses without a socket; production wiring leaves it nil and gets fns.
type DoFunc func(
	ctx context.Context,
	req fns.Request,
) (fns.Response, error)

// Config describes one provider. Do and Now default to the process HTTP client
// and clock when left nil.
type Config struct {
	Host       string
	SearchURL  string
	SearchKind string
	Timeout    time.Duration
	Do         DoFunc
	Now        func() time.Time
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

	tr := newTransport(cfg)

	switch cfg.SearchKind {
	case metadata.SearchKindGitHub:
		return &githubProvider{transport: tr, searchURL: cfg.SearchURL}, nil
	case metadata.SearchKindGitLab:
		return &gitlabProvider{transport: tr, searchURL: cfg.SearchURL}, nil
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

	providers := make([]Provider, 0, len(hosts))
	for _, host := range hosts {
		cfg := base
		cfg.Host = host
		cfg.SearchURL = platforms[host].SearchURL
		cfg.SearchKind = platforms[host].SearchKind

		p, err := New(cfg)
		if err != nil {
			return nil, fmt.Errorf("provider: build %s: %w", host, err)
		}
		providers = append(providers, p)
	}
	return providers, nil
}

func candidateOf(
	host string,
	path string,
	name string,
	description string,
	stars int,
	branch string,
) (Candidate, bool) {
	ns := domain.Namespace(host + domain.NamespaceSeparator + path)
	if ns.Validate() != nil || ns.IsQuiverHosted() {
		return Candidate{}, false
	}

	return Candidate{
		Namespace:     ns,
		Name:          name,
		Description:   description,
		Stars:         stars,
		Source:        host,
		DefaultBranch: branch,
	}, true
}

func truncate(
	candidates []Candidate,
	limit int,
) []Candidate {
	if limit > 0 && len(candidates) > limit {
		return candidates[:limit]
	}
	return candidates
}
