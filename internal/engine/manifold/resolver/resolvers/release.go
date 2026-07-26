package resolvers

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	fnsconfig "github.com/rabbytesoftware/quiver.core/internal/core/fns/config"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// tagPathMarker precedes the ref in a release permalink's redirect target.
// A Location that lacks it points at the release index instead of a release,
// which is how a host says "no stable release".
const (
	tagPathMarker         = "/releases/tag/"
	defaultReleaseTimeout = 30 * time.Second
)

// ReleaseResolver asks a git host which ref its latest stable release carries.
type ReleaseResolver interface {
	Latest(
		ctx context.Context,
		ns domain.Namespace,
	) (string, error)
}

type releaseResolver struct {
	platforms metadata.Platforms
	timeout   time.Duration
}

// NewReleaseResolver builds a resolver over the given platform table. Platforms
// without a LatestReleaseURL always miss.
func NewReleaseResolver(
	platforms metadata.Platforms,
	timeout time.Duration,
) ReleaseResolver {
	if timeout == 0 {
		timeout = defaultReleaseTimeout
	}

	return &releaseResolver{
		platforms: platforms,
		timeout:   timeout,
	}
}

// Latest follows the platform's latest-release permalink for its redirect only.
// The redirect target is a plain web page, not an API endpoint, so no quota is
// consumed and the body is never needed.
func (r *releaseResolver) Latest(
	ctx context.Context,
	ns domain.Namespace,
) (string, error) {
	releaseURL, err := r.releaseURL(ns)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	resp, err := fns.Do(ctx, fns.Request{URL: releaseURL}, fnsconfig.WithoutRedirects())
	if err != nil {
		return "", fmt.Errorf("%w: GET %s: %v", ErrNoLatestRelease, releaseURL, err)
	}

	if resp.Status < 300 || resp.Status >= 400 {
		return "", fmt.Errorf("%w: %s answered HTTP %d", ErrNoLatestRelease, releaseURL, resp.Status)
	}

	ref, ok := refFromLocation(resp.Headers.Get("Location"))
	if !ok {
		return "", fmt.Errorf("%w: %s redirects outside %s", ErrNoLatestRelease, releaseURL, tagPathMarker)
	}

	return ref, nil
}

func (r *releaseResolver) releaseURL(
	ns domain.Namespace,
) (string, error) {
	if err := ns.BareNamespace().Validate(); err != nil {
		return "", fmt.Errorf("%w: %v", ErrNoLatestRelease, err)
	}

	platform, ok := r.platforms[ns.Domain()]
	if !ok || platform.LatestReleaseURL == "" {
		return "", fmt.Errorf("%w: %s declares no release permalink", ErrNoLatestRelease, ns.Domain())
	}

	parts := strings.Split(string(ns.BareNamespace()), domain.NamespaceSeparator)
	return buildReleaseURL(platform.LatestReleaseURL, parts[1], parts[2]), nil
}

// refFromLocation extracts the ref from a release permalink's redirect target.
// The tag segment is path-escaped, so a ref containing "/" arrives encoded.
func refFromLocation(location string) (string, bool) {
	idx := strings.Index(location, tagPathMarker)
	if idx < 0 {
		return "", false
	}

	raw := location[idx+len(tagPathMarker):]
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

func buildReleaseURL(
	template, user, repo string,
) string {
	return strings.NewReplacer(
		"{user}", user,
		"{repo}", repo,
	).Replace(template)
}
