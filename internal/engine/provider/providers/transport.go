package providers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
)

type transport struct {
	host    string
	timeout time.Duration
	do      DoFunc
	now     func() time.Time
}

func newTransport(
	cfg Config,
) transport {
	do := cfg.Do
	if do == nil {
		do = defaultDo
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	return transport{
		host:    cfg.Host,
		timeout: cfg.Timeout,
		do:      do,
		now:     now,
	}
}

func defaultDo(
	ctx context.Context,
	req fns.Request,
) (fns.Response, error) {
	return fns.Do(ctx, req)
}

// get issues one bounded GET and returns the body only for a 2xx. Every other
// status is classified so the caller can distinguish a rate limit from a bad
// a rate limit from a broken host.
func (t transport) get(
	ctx context.Context,
	rawURL string,
	headers http.Header,
) ([]byte, error) {
	if t.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.timeout)
		defer cancel()
	}

	resp, err := t.do(ctx, fns.Request{
		Method:  http.MethodGet,
		URL:     rawURL,
		Headers: headers,
	})
	if err != nil {
		return nil, fmt.Errorf("provider %s: search: %w", t.host, err)
	}

	if err := t.classify(resp); err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (t transport) classify(
	resp fns.Response,
) error {
	if resp.Status >= http.StatusOK && resp.Status < http.StatusMultipleChoices {
		return nil
	}

	if resp.Status == http.StatusTooManyRequests || t.isExhausted(resp) {
		return &RateLimitedError{Host: t.host, RetryAfter: t.retryAfter(resp.Headers)}
	}
	if resp.Status == http.StatusUnauthorized || resp.Status == http.StatusForbidden {
		return &UnauthorizedError{Host: t.host}
	}
	return fmt.Errorf("provider %s: search: http %d", t.host, resp.Status)
}

// isExhausted reads the budget header a 403 carries when the refusal is a rate
// limit rather than a credentials problem.
func (t transport) isExhausted(
	resp fns.Response,
) bool {
	return resp.Status == http.StatusForbidden &&
		resp.Headers.Get("X-RateLimit-Remaining") == "0"
}

// retryAfter prefers the explicit Retry-After header, falling back to the
// absolute reset stamp. A stamp already in the past clamps to zero rather than
// becoming a negative wait.
func (t transport) retryAfter(
	headers http.Header,
) time.Duration {
	if raw := headers.Get("Retry-After"); raw != "" {
		return t.parseRetryAfter(raw)
	}

	reset, err := strconv.ParseInt(headers.Get("X-RateLimit-Reset"), 10, 64)
	if err != nil {
		return 0
	}
	return clampDuration(time.Unix(reset, 0).Sub(t.now()))
}

func (t transport) parseRetryAfter(
	raw string,
) time.Duration {
	if seconds, err := strconv.Atoi(raw); err == nil {
		return clampDuration(time.Duration(seconds) * time.Second)
	}
	if at, err := http.ParseTime(raw); err == nil {
		return clampDuration(at.Sub(t.now()))
	}
	return 0
}

func clampDuration(
	d time.Duration,
) time.Duration {
	if d < 0 {
		return 0
	}
	return d
}

// withLimit appends the page size the host understands, leaving the URL alone
// when no limit was asked for.
func withLimit(
	rawURL string,
	limit int,
) string {
	if limit <= 0 {
		return rawURL
	}

	separator := "?"
	if strings.Contains(rawURL, "?") {
		separator = "&"
	}
	return rawURL + separator + "per_page=" + strconv.Itoa(limit)
}

func buildSearchURL(
	template string,
	query string,
	topic string,
) string {
	return strings.NewReplacer(
		"{query}", url.QueryEscape(query),
		"{topic}", url.QueryEscape(topic),
	).Replace(template)
}
