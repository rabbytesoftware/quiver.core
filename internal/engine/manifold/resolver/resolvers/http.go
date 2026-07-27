package resolvers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

// httpFetcher reads a manifest over HTTP from wherever its host serves raw
// files. Which URL that is, and which refs to fall back on, are the host's
// answers: this fetcher only decides which of them to try and what a 404 means.
type httpFetcher struct {
	hosts hosts.Lookup
}

func NewHTTP(
	lookup hosts.Lookup,
) Fetcher {
	return &httpFetcher{
		hosts: hosts.Or(lookup),
	}
}

func (h *httpFetcher) CanResolve(
	namespace domain.Namespace,
) bool {
	_, ok := h.hosts(namespace)
	return ok
}

func (h *httpFetcher) Fetch(
	ctx context.Context,
	namespace domain.Namespace,
	filePath string,
	timeout time.Duration,
) ([]byte, error) {
	host, ok := h.hosts(namespace)
	if !ok {
		return nil, fmt.Errorf("%w: no host serves %s", ErrNotFound, namespace.Domain())
	}

	branches := host.DefaultBranches()
	if ref := namespace.Ref(); ref != "" {
		branches = []string{ref}
	}

	return h.fetchBranches(ctx, host, namespace, filePath, branches, timeout)
}

// fetchBranches walks the candidate branches in order. Only a 404 is evidence
// that a branch does not carry the file, so any other failure aborts instead of
// masking itself as a missing branch.
func (h *httpFetcher) fetchBranches(
	ctx context.Context,
	host hosts.Host,
	namespace domain.Namespace,
	filePath string,
	branches []string,
	timeout time.Duration,
) ([]byte, error) {
	var lastErr error

	for _, branch := range branches {
		rawURL, err := host.RawFileURL(namespace, branch, filePath)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrFetchFailed, err)
		}

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
	return nil, fmt.Errorf("%w: %s has no candidate branch", ErrNotFound, namespace.Domain())
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
