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
	fnsconfig "github.com/rabbytesoftware/quiver.core/internal/core/fns/config"
)

type transport struct {
	host    string
	timeout time.Duration
	do      DoFunc
	// follow issues the request whose answer is the redirect itself, so it must
	// not follow one. A test that supplies Do owns both: its canned response is
	// already whatever the host would have replied.
	follow DoFunc
	now    func() time.Time
}

func newTransport(
	cfg Config,
) transport {
	do, follow := cfg.Do, cfg.Do
	if do == nil {
		do, follow = defaultDo, redirectDo
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	return transport{
		host:    cfg.Host,
		timeout: cfg.Timeout,
		do:      do,
		follow:  follow,
		now:     now,
	}
}

func defaultDo(
	ctx context.Context,
	req fns.Request,
) (fns.Response, error) {
	return fns.Do(ctx, req)
}

// redirectDo hands back the 3xx itself rather than what it points at: the
// Location header is the whole answer, and following it would spend a second
// request on a page nobody reads.
func redirectDo(
	ctx context.Context,
	req fns.Request,
) (fns.Response, error) {
	return fns.Do(ctx, req, fnsconfig.WithoutRedirects())
}

// get issues one bounded GET and returns the body only for a 2xx. Every other
// status is classified so the caller can distinguish a rate limit from a bad
// a rate limit from a broken host.
func (t transport) get(
	ctx context.Context,
	rawURL string,
	headers http.Header,
) ([]byte, error) {
	ctx, cancel := t.bound(ctx)
	defer cancel()

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

// redirect issues one bounded GET and returns the response unclassified: this
// exchange succeeds with a 3xx, which every other request treats as a failure.
func (t transport) redirect(
	ctx context.Context,
	rawURL string,
) (fns.Response, error) {
	ctx, cancel := t.bound(ctx)
	defer cancel()

	resp, err := t.follow(ctx, fns.Request{
		Method: http.MethodGet,
		URL:    rawURL,
	})
	if err != nil {
		return fns.Response{}, err
	}
	return resp, nil
}

// bound applies the provider's own timeout, so a hung host cannot hold a whole
// discovery pass open. A zero timeout leaves the caller's deadline alone.
func (t transport) bound(
	ctx context.Context,
) (context.Context, context.CancelFunc) {
	if t.timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, t.timeout)
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
