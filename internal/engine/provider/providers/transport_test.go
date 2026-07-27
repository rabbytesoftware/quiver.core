package providers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
)

// ─── rate limiting ───────────────────────────────────────────────────────────

// A rate-limited provider must be distinguishable from one that simply found
// nothing, so the caller can say "GitHub is rate-limited, try in 40s".
func TestTransport_RateLimited403(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-RateLimit-Remaining", "0")
	headers.Set("X-RateLimit-Reset", strconv.FormatInt(fixedNow().Add(40*time.Second).Unix(), 10))

	stub := &stubDoer{response: fns.Response{Status: http.StatusForbidden, Headers: headers}}

	_, err := newGitHub(stub).Search(context.Background(), SearchRequest{Text: "x"})
	require.Error(t, err)

	var limited *RateLimitedError
	require.ErrorAs(t, err, &limited)
	assert.Equal(t, "github.com", limited.Host)
	assert.Equal(t, 40*time.Second, limited.RetryAfter)
	assert.Contains(t, limited.Error(), "rate limited")
}

func TestTransport_RetryAfter(t *testing.T) {
	testCases := []struct {
		name   string
		header string
		value  string
		want   time.Duration
	}{
		{name: "seconds", header: "Retry-After", value: "17", want: 17 * time.Second},
		{
			name:   "http date",
			header: "Retry-After",
			value:  fixedNow().Add(90 * time.Second).Format(http.TimeFormat),
			want:   90 * time.Second,
		},
		{name: "unparseable", header: "Retry-After", value: "later", want: 0},
		{name: "negative seconds clamp to zero", header: "Retry-After", value: "-5", want: 0},
		{
			name:   "http date in the past clamps to zero",
			header: "Retry-After",
			value:  fixedNow().Add(-time.Minute).Format(http.TimeFormat),
			want:   0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			headers := http.Header{}
			headers.Set(tc.header, tc.value)

			stub := &stubDoer{response: fns.Response{Status: http.StatusTooManyRequests, Headers: headers}}

			_, err := newGitHub(stub).Search(context.Background(), SearchRequest{Text: "x"})

			var limited *RateLimitedError
			require.ErrorAs(t, err, &limited)
			assert.Equal(t, tc.want, limited.RetryAfter)
		})
	}
}

func TestTransport_RateLimited429WithoutAnyHint(t *testing.T) {
	stub := &stubDoer{response: fns.Response{Status: http.StatusTooManyRequests, Headers: http.Header{}}}

	_, err := newGitHub(stub).Search(context.Background(), SearchRequest{Text: "x"})

	var limited *RateLimitedError
	require.ErrorAs(t, err, &limited)
	assert.Zero(t, limited.RetryAfter)
}

// A reset stamp already in the past must not become a negative wait.
func TestTransport_RateLimitReset(t *testing.T) {
	testCases := []struct {
		name  string
		reset string
		want  time.Duration
	}{
		{name: "in the past clamps to zero", reset: strconv.FormatInt(fixedNow().Add(-time.Minute).Unix(), 10)},
		{name: "unparseable is zero", reset: "soon"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			headers := http.Header{}
			headers.Set("X-RateLimit-Remaining", "0")
			headers.Set("X-RateLimit-Reset", tc.reset)

			stub := &stubDoer{response: fns.Response{Status: http.StatusForbidden, Headers: headers}}

			_, err := newGitHub(stub).Search(context.Background(), SearchRequest{Text: "x"})

			var limited *RateLimitedError
			require.ErrorAs(t, err, &limited)
			assert.Equal(t, tc.want, limited.RetryAfter)
		})
	}
}

// ─── authorization ───────────────────────────────────────────────────────────

func TestTransport_Unauthorized401(t *testing.T) {
	stub := &stubDoer{response: fns.Response{Status: http.StatusUnauthorized, Headers: http.Header{}}}

	_, err := newGitHub(stub).Search(context.Background(), SearchRequest{Text: "x"})

	var unauthorized *UnauthorizedError
	require.ErrorAs(t, err, &unauthorized)
	assert.Equal(t, "github.com", unauthorized.Host)
	assert.Contains(t, unauthorized.Error(), "unauthorized")

	var limited *RateLimitedError
	assert.False(t, errors.As(err, &limited), "a bad token is not a rate limit")
}

// A 403 without the remaining-zero header is a bad or missing token, not a
// rate limit.
func TestTransport_Forbidden403WithoutRateLimitHeader_IsUnauthorized(t *testing.T) {
	stub := &stubDoer{response: fns.Response{Status: http.StatusForbidden, Headers: http.Header{}}}

	_, err := newGitHub(stub).Search(context.Background(), SearchRequest{Text: "x"})

	var unauthorized *UnauthorizedError
	require.ErrorAs(t, err, &unauthorized)
}

// ─── other failures ──────────────────────────────────────────────────────────

func TestTransport_ServerError_IsGenericError(t *testing.T) {
	stub := &stubDoer{response: fns.Response{Status: http.StatusInternalServerError, Headers: http.Header{}}}

	_, err := newGitHub(stub).Search(context.Background(), SearchRequest{Text: "x"})
	require.Error(t, err)

	var limited *RateLimitedError
	assert.False(t, errors.As(err, &limited))
	var unauthorized *UnauthorizedError
	assert.False(t, errors.As(err, &unauthorized))
	assert.Contains(t, err.Error(), "500")
}

func TestTransport_NetworkError(t *testing.T) {
	stub := &stubDoer{err: errors.New("dial tcp: connection refused")}

	_, err := newGitHub(stub).Search(context.Background(), SearchRequest{Text: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

// ─── timeout and cancellation ────────────────────────────────────────────────

func TestTransport_CancelledContext_ReturnsPromptly(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	entered := make(chan struct{})
	blocking := func(ctx context.Context, _ fns.Request) (fns.Response, error) {
		close(entered)
		select {
		case <-ctx.Done():
			return fns.Response{}, ctx.Err()
		case <-release:
			return okBody(githubPayload), nil
		}
	}

	cfg := githubConfig(&stubDoer{})
	cfg.Do = blocking
	p := NewGitHub(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := p.Search(ctx, SearchRequest{Text: "x"})
		done <- err
	}()

	<-entered
	cancel()

	err := <-done
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// The provider timeout bounds one search, so a hung host cannot hold the whole
// discovery pass open.
func TestTransport_AppliesItsOwnTimeout(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	blocking := func(ctx context.Context, _ fns.Request) (fns.Response, error) {
		select {
		case <-ctx.Done():
			return fns.Response{}, ctx.Err()
		case <-release:
			return okBody(githubPayload), nil
		}
	}

	cfg := githubConfig(&stubDoer{})
	cfg.Do = blocking
	cfg.Timeout = time.Nanosecond

	_, err := NewGitHub(cfg).Search(context.Background(), SearchRequest{Text: "x"})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestTransport_ZeroTimeout_DoesNotBoundTheRequest(t *testing.T) {
	stub := &stubDoer{response: okBody(githubPayload)}
	cfg := githubConfig(stub)
	cfg.Timeout = 0

	got, err := NewGitHub(cfg).Search(context.Background(), SearchRequest{Text: "x"})
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestTransport_NilDo_UsesTheDefaultTransport(t *testing.T) {
	p := NewGitHub(Config{
		Host:      "github.com",
		SearchURL: "https://api.github.com/search/repositories?q={query}",
		Timeout:   time.Nanosecond,
	})

	// The nanosecond deadline expires before any socket is opened, so this
	// exercises the default doer without touching the network.
	_, err := p.Search(context.Background(), SearchRequest{Text: "x"})
	require.Error(t, err)
}

// ─── error messages ──────────────────────────────────────────────────────────

func TestRateLimitedError_Error(t *testing.T) {
	testCases := []struct {
		name string
		err  *RateLimitedError
		want string
	}{
		{
			name: "without retry after",
			err:  &RateLimitedError{Host: "github.com"},
			want: "github.com is rate limited",
		},
		{
			name: "with retry after",
			err:  &RateLimitedError{Host: "github.com", RetryAfter: 40 * time.Second},
			want: "github.com is rate limited, retry in 40s",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.err.Error())
		})
	}
}
