package gather

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	fnsconfig "github.com/rabbytesoftware/quiver.core/internal/core/fns/config"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/media"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
)

const (
	maxPageBytes  = 1 << 20
	maxProbeBytes = 64 << 10
)

type fetcher struct {
	timeout time.Duration
}

func (f fetcher) document(
	ctx context.Context,
	rawURL string,
) ([]byte, error) {
	if err := requireHTTP(rawURL); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	resp, err := fns.Do(ctx, fns.Request{URL: rawURL}, contextBound()...)
	if err != nil {
		return nil, fmt.Errorf("%w: GET %s: %w", resolver.ErrFetchFailed, rawURL, err)
	}
	if resp.Status == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s", resolver.ErrNotFound, rawURL)
	}
	if resp.Status < http.StatusOK || resp.Status >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: %s answered http %d", resolver.ErrFetchFailed, rawURL, resp.Status)
	}
	return resp.Body, nil
}

func (f fetcher) prefixOf(
	limit int64,
) media.Fetch {
	return func(ctx context.Context, rawURL string) ([]byte, error) {
		return f.prefix(ctx, rawURL, limit)
	}
}

func (f fetcher) prefix(
	ctx context.Context,
	rawURL string,
	limit int64,
) ([]byte, error) {
	if err := requireHTTP(rawURL); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	body, err := fns.DownloadStream(ctx, rawURL, nil, contextBound()...)
	if err != nil {
		return nil, fmt.Errorf("%w: GET %s: %w", resolver.ErrFetchFailed, rawURL, err)
	}
	defer body.Close() //nolint:errcheck

	data, err := io.ReadAll(io.LimitReader(body, limit))
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %w", resolver.ErrFetchFailed, rawURL, err)
	}
	return data, nil
}

func contextBound() []fnsconfig.Option {
	return []fnsconfig.Option{fnsconfig.WithTimeout(0)}
}

func requireHTTP(
	rawURL string,
) error {
	isHTTP := strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://")
	parsed, err := url.Parse(rawURL)
	if !isHTTP || err != nil || parsed.Host == "" {
		return fmt.Errorf("%w: not an http url: %q", resolver.ErrFetchFailed, rawURL)
	}
	return nil
}
