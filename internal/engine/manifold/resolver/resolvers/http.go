package resolvers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rabbytesoftware/quiver/internal/core/metadata"
	"github.com/rabbytesoftware/quiver/internal/domain"
)

type httpFetcher struct {
	platforms metadata.Platforms
}

func NewHTTP(platforms metadata.Platforms) Fetcher {
	return &httpFetcher{platforms: platforms}
}

func (h *httpFetcher) CanResolve(namespace domain.Namespace) bool {
	parts := strings.SplitN(string(namespace), domain.NamespaceSeparator, 2)
	_, ok := h.platforms[parts[0]]
	return ok
}

func (h *httpFetcher) Fetch(
	ctx context.Context,
	namespace domain.Namespace,
	filePath string,
	timeout time.Duration,
) ([]byte, error) {
	parts := strings.Split(string(namespace), domain.NamespaceSeparator)
	platform := h.platforms[parts[0]]
	rawURL := buildRawURL(platform.RawURL, parts[1], parts[2], platform.DefaultBranch, filePath)
	return fetchHTTP(ctx, rawURL, timeout)
}

func fetchHTTP(ctx context.Context, rawURL string, timeout time.Duration) ([]byte, error) {
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
	defer resp.Body.Close()

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

func buildRawURL(template, user, repo, branch, file string) string {
	r := strings.NewReplacer(
		"{user}", user,
		"{repo}", repo,
		"{branch}", branch,
		"{file}", file,
	)
	return r.Replace(template)
}
