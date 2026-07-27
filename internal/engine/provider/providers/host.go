package providers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// host answers everything a git host can answer from its own entry: where it
// serves a raw file, which refs it defaults to, and which ref its latest
// release carries. Every provider embeds it.
//
// Search is not one of those answers. A host with a search API implements it;
// the rest inherit the refusal here, because searching is a capability some
// hosts have and none needs in order to serve a manifest.
type host struct {
	name            string
	rawURL          string
	defaultBranches []string
	releaseURL      string
	// releaseMarker precedes the ref in the redirect the release permalink
	// answers with, and is the one piece of that exchange that differs per
	// host. A host that publishes no releases leaves it empty.
	releaseMarker string
	transport     transport
}

func newHost(
	cfg Config,
	releaseMarker string,
) host {
	return host{
		name:            cfg.Host,
		rawURL:          cfg.RawURL,
		defaultBranches: cfg.DefaultBranches,
		releaseURL:      cfg.LatestReleaseURL,
		releaseMarker:   releaseMarker,
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

// LatestRelease follows the host's latest-release permalink for its redirect
// only. The redirect target is a plain web page, not an API endpoint, so no
// quota is consumed and the body is never needed.
func (h host) LatestRelease(
	ctx context.Context,
	ns domain.Namespace,
) (string, error) {
	releaseURL, err := h.releaseURLFor(ns)
	if err != nil {
		return "", err
	}

	resp, err := h.transport.redirect(ctx, releaseURL)
	if err != nil {
		return "", fmt.Errorf("%w: GET %s: %v", ErrNoLatestRelease, releaseURL, err)
	}

	if resp.Status < http.StatusMultipleChoices || resp.Status >= http.StatusBadRequest {
		return "", fmt.Errorf("%w: %s answered http %d", ErrNoLatestRelease, releaseURL, resp.Status)
	}

	ref, ok := refFromLocation(resp.Headers.Get("Location"), h.releaseMarker)
	if !ok {
		return "", fmt.Errorf("%w: %s redirects outside %s", ErrNoLatestRelease, releaseURL, h.releaseMarker)
	}

	return ref, nil
}

func (h host) releaseURLFor(
	ns domain.Namespace,
) (string, error) {
	if h.releaseURL == "" || h.releaseMarker == "" {
		return "", fmt.Errorf("%w: %s publishes no releases", ErrNoLatestRelease, h.name)
	}

	user, repo, err := repositoryOf(ns)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNoLatestRelease, err)
	}

	return strings.NewReplacer(
		"{user}", user,
		"{repo}", repo,
	).Replace(h.releaseURL), nil
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

// refFromLocation extracts the ref from a release permalink's redirect target.
// A Location that lacks the marker points at the release index instead of a
// release, which is how a host says "no stable release". The tag segment is
// path-escaped, so a ref containing "/" arrives encoded.
func refFromLocation(
	location string,
	marker string,
) (string, bool) {
	idx := strings.Index(location, marker)
	if idx < 0 {
		return "", false
	}

	raw := location[idx+len(marker):]
	if cut := strings.IndexAny(raw, "?#"); cut >= 0 {
		raw = raw[:cut]
	}

	ref, err := url.PathUnescape(raw)
	if err != nil {
		return "", false
	}

	ref = strings.Trim(ref, "/")
	if ref == "" {
		return "", false
	}

	return ref, true
}
