package providers

import (
	"context"
	"fmt"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// host answers everything a git host can answer from its own entry: where it
// serves a raw file and which refs it defaults to. Every provider embeds it.
//
// Search is not one of those answers. A host with a search API implements it;
// the rest inherit the refusal here, because searching is a capability some
// hosts have and none needs in order to serve a manifest.
type host struct {
	name            string
	rawURL          string
	defaultBranches []string
	transport       transport
}

func newHost(
	cfg Config,
) host {
	return host{
		name:            cfg.Host,
		rawURL:          cfg.RawURL,
		defaultBranches: cfg.DefaultBranches,
		transport:       newTransport(cfg),
	}
}

func (h host) Host() string {
	return h.name
}

// CanSearch is false for a plain host: only the hosts with a search dialect of
// their own override it.
func (h host) CanSearch() bool {
	return false
}

func (h host) Search(
	_ context.Context,
	_ SearchRequest,
) ([]Candidate, error) {
	return nil, fmt.Errorf("provider %s: %w", h.name, ErrSearchUnsupported)
}

func (h host) DefaultBranches() []string {
	return h.defaultBranches
}

// RawFileURL names where file lives at ref, without fetching it: reading a
// manifest is manifold's job, and knowing the URL shape is this one's.
func (h host) RawFileURL(
	ns domain.Namespace,
	ref string,
	file string,
) (string, error) {
	user, repo, err := repositoryOf(ns)
	if err != nil {
		return "", fmt.Errorf("provider %s: raw file url: %w", h.name, err)
	}

	if h.rawURL == "" {
		return "", fmt.Errorf("provider %s: %w", h.name, ErrNoRawURL)
	}

	return strings.NewReplacer(
		"{user}", user,
		"{repo}", repo,
		"{branch}", ref,
		"{file}", file,
	).Replace(h.rawURL), nil
}

// repositoryOf splits a namespace into the two segments every host addresses a
// repository by. The ref is dropped: it names a revision, not a repository.
func repositoryOf(
	ns domain.Namespace,
) (string, string, error) {
	bare := ns.BareNamespace()
	if err := bare.Validate(); err != nil {
		return "", "", err
	}

	segments := strings.Split(string(bare), domain.NamespaceSeparator)
	return segments[1], segments[2], nil
}
