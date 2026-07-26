package resolvers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type httpFetcher struct {
	platforms metadata.Platforms
}

func NewHTTP(
	platforms metadata.Platforms,
) Fetcher {
	return &httpFetcher{
		platforms: platforms,
	}
}

func (h *httpFetcher) CanResolve(
	namespace domain.Namespace,
) bool {
	_, ok := h.platforms[namespace.Domain()]
	return ok
}

func (h *httpFetcher) Fetch(
	ctx context.Context,
	namespace domain.Namespace,
	filePath string,
	timeout time.Duration,
) ([]byte, error) {
	parts := strings.Split(string(namespace.BareNamespace()), domain.NamespaceSeparator)
	platform := h.platforms[parts[0]]

	branches := platform.DefaultBranches
	if ref := namespace.Ref(); ref != "" {
		branches = []string{ref}
	}

	return h.fetchBranches(ctx, platform.RawURL, parts, filePath, branches, timeout)
}

// fetchBranches walks the candidate branches in order. Only a 404 is evidence
// that a branch does not carry the file, so any other failure aborts instead of
// masking itself as a missing branch.
func (h *httpFetcher) fetchBranches(
	ctx context.Context,
	template string,
	parts []string,
	filePath string,
	branches []string,
	timeout time.Duration,
) ([]byte, error) {
	var lastErr error

	for _, branch := range branches {
		rawURL := buildRawURL(template, parts[1], parts[2], branch, filePath)

		data, err := fetchHTTP(ctx, rawURL, timeout)
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("%w: %s has no candidate branch", ErrNotFound, parts[0])
}

func fetchHTTP(
	ctx context.Context,
	rawURL string,
	timeout time.Duration,
) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrFetchFailed, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: GET %s: %v", ErrFetchFailed, rawURL, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, rawURL)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: HTTP %d: %s", ErrFetchFailed, resp.StatusCode, rawURL)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrFetchFailed, err)
	}

	return data, nil
}

func buildRawURL(
	template, user, repo, branch, file string,
) string {
	r := strings.NewReplacer(
		"{user}", user,
		"{repo}", repo,
		"{branch}", branch,
		"{file}", file,
	)
	return r.Replace(template)
}
