package providers

import (
	"context"
	"fmt"
	"net/url"
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
	blobURL         string
	repoPageURL     string
	defaultBranches []string
	transport       transport
}

func newHost(
	cfg Config,
) host {
	return host{
		name:            cfg.Host,
		rawURL:          cfg.RawURL,
		blobURL:         cfg.BlobURL,
		repoPageURL:     cfg.RepoPageURL,
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

func (h host) BlobFileURL(
	ns domain.Namespace,
	ref string,
	file string,
) (string, error) {
	if h.blobURL == "" {
		return "", fmt.Errorf("provider %s: %w", h.name, ErrNoBlobURL)
	}

	blob, err := fillRepoTemplate(h.blobURL, ns, ref, file)
	if err != nil {
		return "", fmt.Errorf("provider %s: blob file url: %w", h.name, err)
	}
	return blob, nil
}

func (h host) RepoPageURL(
	ns domain.Namespace,
) string {
	if h.repoPageURL == "" {
		return ""
	}

	page, err := fillRepoTemplate(h.repoPageURL, ns, "", "")
	if err != nil {
		return ""
	}
	return page
}

func fillRepoTemplate(
	template string,
	ns domain.Namespace,
	ref string,
	file string,
) (string, error) {
	user, repo, err := repositoryOf(ns)
	if err != nil {
		return "", err
	}

	return strings.NewReplacer(
		"{user}", user,
		"{repo}", repo,
		"{branch}", ref,
		"{file}", file,
	).Replace(template), nil
}

func (h host) ReleaseAssets(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) ([]domain.ReleaseAsset, error) {
	return []domain.ReleaseAsset{}, nil
}

func tagURL(
	template string,
	ns domain.Namespace,
	tag string,
) (string, error) {
	if template == "" {
		return "", ErrNoRawURL
	}

	user, repo, err := repositoryOf(ns)
	if err != nil {
		return "", err
	}

	return strings.NewReplacer(
		"{user}", url.PathEscape(user),
		"{repo}", url.PathEscape(repo),
		"{tag}", url.PathEscape(tag),
	).Replace(template), nil
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
