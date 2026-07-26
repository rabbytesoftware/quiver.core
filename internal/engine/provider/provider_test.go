package provider_test

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
)

// ─── fns stub ────────────────────────────────────────────────────────────────

// stubDoer records every outgoing request so tests can assert on the URL and
// headers, not only on what came back.
type stubDoer struct {
	mu       sync.Mutex
	requests []fns.Request
	response fns.Response
	err      error
}

func (s *stubDoer) do(
	_ context.Context,
	req fns.Request,
) (fns.Response, error) {
	s.mu.Lock()
	s.requests = append(s.requests, req)
	s.mu.Unlock()
	return s.response, s.err
}

func (s *stubDoer) lastURL(t *testing.T) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	require.Len(t, s.requests, 1)
	return s.requests[0].URL
}

func (s *stubDoer) lastHeaders(t *testing.T) http.Header {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	require.Len(t, s.requests, 1)
	return s.requests[0].Headers
}

func okBody(body string) fns.Response {
	return fns.Response{Status: http.StatusOK, Headers: http.Header{}, Body: []byte(body)}
}

func fixedNow() time.Time {
	return time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
}

func newProvider(
	t *testing.T,
	kind string,
	stub *stubDoer,
	mutate func(*provider.Config),
) provider.Provider {
	t.Helper()

	cfg := provider.Config{
		Host:       "github.com",
		SearchURL:  "https://api.github.com/search/repositories?q={query}",
		SearchKind: kind,
		Timeout:    5 * time.Second,
		Do:         stub.do,
		Now:        fixedNow,
	}
	if kind == metadata.SearchKindGitLab {
		cfg.Host = "gitlab.com"
		cfg.SearchURL = "https://gitlab.com/api/v4/projects?search={query}&topic={topic}"
	}
	if mutate != nil {
		mutate(&cfg)
	}

	p, err := provider.New(cfg)
	require.NoError(t, err)
	return p
}

func newGitHub(
	t *testing.T,
	stub *stubDoer,
) provider.Provider {
	t.Helper()
	return newProvider(t, metadata.SearchKindGitHub, stub, nil)
}

const githubPayload = `{
  "total_count": 2,
  "items": [
    {
      "full_name": "acme/chromium",
      "name": "chromium",
      "description": "A fast browser",
      "stargazers_count": 42,
      "default_branch": "master"
    },
    {
      "full_name": "acme/firefox",
      "name": "firefox",
      "description": "Another browser",
      "stargazers_count": 7,
      "default_branch": "main"
    }
  ]
}`

const gitlabPayload = `[
  {
    "path_with_namespace": "acme/chromium",
    "name": "chromium",
    "description": "A fast browser",
    "star_count": 42,
    "default_branch": "master"
  }
]`

// ─── construction ────────────────────────────────────────────────────────────

func TestNew_UnknownKind_ReturnsError(t *testing.T) {
	_, err := provider.New(provider.Config{
		Host:       "example.com",
		SearchURL:  "https://example.com?q={query}",
		SearchKind: "bitbucket",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown search kind")
}

func TestNew_EmptySearchURL_ReturnsError(t *testing.T) {
	_, err := provider.New(provider.Config{
		Host:       "bitbucket.org",
		SearchKind: metadata.SearchKindGitHub,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no search url")
}

func TestNew_EmptyHost_ReturnsError(t *testing.T) {
	_, err := provider.New(provider.Config{
		SearchURL:  "https://api.github.com/search/repositories?q={query}",
		SearchKind: metadata.SearchKindGitHub,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no host")
}

func TestProvider_Host_ReturnsConfiguredHost(t *testing.T) {
	assert.Equal(t, "github.com", newGitHub(t, &stubDoer{response: okBody(githubPayload)}).Host())
	assert.Equal(
		t,
		"gitlab.com",
		newProvider(t, metadata.SearchKindGitLab, &stubDoer{response: okBody(gitlabPayload)}, nil).Host(),
	)
}

// ─── GitHub ──────────────────────────────────────────────────────────────────

func TestGitHub_Search_ParsesCandidates(t *testing.T) {
	stub := &stubDoer{response: okBody(githubPayload)}

	got, err := newGitHub(t, stub).Search(context.Background(), provider.SearchRequest{
		Text:   "browser",
		Topics: []string{"quiver-arrow"},
		Limit:  25,
	})
	require.NoError(t, err)
	require.Len(t, got, 2)

	assert.Equal(t, provider.Candidate{
		Namespace:     domain.Namespace("github.com/acme/chromium"),
		Name:          "chromium",
		Description:   "A fast browser",
		Stars:         42,
		Source:        "github.com",
		DefaultBranch: "master",
	}, got[0])
	assert.Equal(t, "main", got[1].DefaultBranch)
	assert.Equal(t, 7, got[1].Stars)
}

func TestGitHub_Search_EmptyResults(t *testing.T) {
	stub := &stubDoer{response: okBody(`{"total_count": 0, "items": []}`)}

	got, err := newGitHub(t, stub).Search(context.Background(), provider.SearchRequest{Text: "nothing"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestGitHub_Search_MalformedJSON(t *testing.T) {
	stub := &stubDoer{response: okBody(`{"items": [`)}

	_, err := newGitHub(t, stub).Search(context.Background(), provider.SearchRequest{Text: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}

func TestGitHub_Search_SkipsRepositoriesWithAnUnusableFullName(t *testing.T) {
	stub := &stubDoer{response: okBody(`{"items": [
		{"full_name": "no-slash", "name": "x", "default_branch": "main"},
		{"full_name": "acme/ok", "name": "ok", "default_branch": "main"}
	]}`)}

	got, err := newGitHub(t, stub).Search(context.Background(), provider.SearchRequest{Text: "x"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, domain.Namespace("github.com/acme/ok"), got[0].Namespace)
}

func TestGitHub_Search_TruncatesToTheRequestedLimit(t *testing.T) {
	stub := &stubDoer{response: okBody(githubPayload)}

	got, err := newGitHub(t, stub).Search(context.Background(), provider.SearchRequest{Text: "browser", Limit: 1})
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

// A rate-limited provider must be distinguishable from one that simply found
// nothing, so the caller can say "GitHub is rate-limited, try in 40s".
func TestGitHub_Search_RateLimited403(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-RateLimit-Remaining", "0")
	headers.Set("X-RateLimit-Reset", strconv.FormatInt(fixedNow().Add(40*time.Second).Unix(), 10))

	stub := &stubDoer{response: fns.Response{Status: http.StatusForbidden, Headers: headers}}

	_, err := newGitHub(t, stub).Search(context.Background(), provider.SearchRequest{Text: "x"})
	require.Error(t, err)

	var limited *provider.RateLimitedError
	require.ErrorAs(t, err, &limited)
	assert.Equal(t, "github.com", limited.Host)
	assert.Equal(t, 40*time.Second, limited.RetryAfter)
	assert.Contains(t, limited.Error(), "rate limited")
}

func TestGitHub_Search_RateLimited429WithRetryAfter(t *testing.T) {
	headers := http.Header{}
	headers.Set("Retry-After", "17")

	stub := &stubDoer{response: fns.Response{Status: http.StatusTooManyRequests, Headers: headers}}

	_, err := newGitHub(t, stub).Search(context.Background(), provider.SearchRequest{Text: "x"})

	var limited *provider.RateLimitedError
	require.ErrorAs(t, err, &limited)
	assert.Equal(t, 17*time.Second, limited.RetryAfter)
}

func TestGitHub_Search_RateLimited429WithHTTPDateRetryAfter(t *testing.T) {
	headers := http.Header{}
	headers.Set("Retry-After", fixedNow().Add(90*time.Second).Format(http.TimeFormat))

	stub := &stubDoer{response: fns.Response{Status: http.StatusTooManyRequests, Headers: headers}}

	_, err := newGitHub(t, stub).Search(context.Background(), provider.SearchRequest{Text: "x"})

	var limited *provider.RateLimitedError
	require.ErrorAs(t, err, &limited)
	assert.Equal(t, 90*time.Second, limited.RetryAfter)
}

func TestGitHub_Search_RateLimited429WithoutAnyHint(t *testing.T) {
	stub := &stubDoer{response: fns.Response{Status: http.StatusTooManyRequests, Headers: http.Header{}}}

	_, err := newGitHub(t, stub).Search(context.Background(), provider.SearchRequest{Text: "x"})

	var limited *provider.RateLimitedError
	require.ErrorAs(t, err, &limited)
	assert.Zero(t, limited.RetryAfter)
}

// A reset stamp already in the past must not become a negative wait.
func TestGitHub_Search_RateLimitResetInThePast_ClampsToZero(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-RateLimit-Remaining", "0")
	headers.Set("X-RateLimit-Reset", strconv.FormatInt(fixedNow().Add(-time.Minute).Unix(), 10))

	stub := &stubDoer{response: fns.Response{Status: http.StatusForbidden, Headers: headers}}

	_, err := newGitHub(t, stub).Search(context.Background(), provider.SearchRequest{Text: "x"})

	var limited *provider.RateLimitedError
	require.ErrorAs(t, err, &limited)
	assert.Zero(t, limited.RetryAfter)
}

func TestGitHub_Search_RateLimitResetUnparseable_IsZero(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-RateLimit-Remaining", "0")
	headers.Set("X-RateLimit-Reset", "soon")

	stub := &stubDoer{response: fns.Response{Status: http.StatusForbidden, Headers: headers}}

	_, err := newGitHub(t, stub).Search(context.Background(), provider.SearchRequest{Text: "x"})

	var limited *provider.RateLimitedError
	require.ErrorAs(t, err, &limited)
	assert.Zero(t, limited.RetryAfter)
}

func TestGitHub_Search_Unauthorized401(t *testing.T) {
	stub := &stubDoer{response: fns.Response{Status: http.StatusUnauthorized, Headers: http.Header{}}}

	_, err := newGitHub(t, stub).Search(context.Background(), provider.SearchRequest{Text: "x"})

	var unauthorized *provider.UnauthorizedError
	require.ErrorAs(t, err, &unauthorized)
	assert.Equal(t, "github.com", unauthorized.Host)
	assert.Contains(t, unauthorized.Error(), "unauthorized")

	var limited *provider.RateLimitedError
	assert.False(t, errors.As(err, &limited), "a bad token is not a rate limit")
}

// A 403 without the remaining-zero header is a bad or missing token, not a
// rate limit.
func TestGitHub_Search_Forbidden403WithoutRateLimitHeader_IsUnauthorized(t *testing.T) {
	stub := &stubDoer{response: fns.Response{Status: http.StatusForbidden, Headers: http.Header{}}}

	_, err := newGitHub(t, stub).Search(context.Background(), provider.SearchRequest{Text: "x"})

	var unauthorized *provider.UnauthorizedError
	require.ErrorAs(t, err, &unauthorized)
}

func TestGitHub_Search_ServerError_IsGenericError(t *testing.T) {
	stub := &stubDoer{response: fns.Response{Status: http.StatusInternalServerError, Headers: http.Header{}}}

	_, err := newGitHub(t, stub).Search(context.Background(), provider.SearchRequest{Text: "x"})
	require.Error(t, err)

	var limited *provider.RateLimitedError
	assert.False(t, errors.As(err, &limited))
	var unauthorized *provider.UnauthorizedError
	assert.False(t, errors.As(err, &unauthorized))
	assert.Contains(t, err.Error(), "500")
}

func TestGitHub_Search_NetworkError(t *testing.T) {
	stub := &stubDoer{err: errors.New("dial tcp: connection refused")}

	_, err := newGitHub(t, stub).Search(context.Background(), provider.SearchRequest{Text: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestGitHub_Search_SendsTopicInQuery(t *testing.T) {
	stub := &stubDoer{response: okBody(githubPayload)}

	_, err := newGitHub(t, stub).Search(context.Background(), provider.SearchRequest{
		Text:   "web browser",
		Topics: []string{"quiver-arrow"},
		Limit:  25,
	})
	require.NoError(t, err)

	assert.Equal(
		t,
		"https://api.github.com/search/repositories?q=web+browser+topic%3Aquiver-arrow&per_page=25",
		stub.lastURL(t),
	)
}

func TestGitHub_Search_NoTopics_StillSendsTheText(t *testing.T) {
	stub := &stubDoer{response: okBody(githubPayload)}

	_, err := newGitHub(t, stub).Search(context.Background(), provider.SearchRequest{Text: "browser", Limit: 5})
	require.NoError(t, err)

	assert.Equal(
		t,
		"https://api.github.com/search/repositories?q=browser&per_page=5",
		stub.lastURL(t),
	)
}

func TestGitHub_Search_ZeroLimit_OmitsPerPage(t *testing.T) {
	stub := &stubDoer{response: okBody(githubPayload)}

	_, err := newGitHub(t, stub).Search(context.Background(), provider.SearchRequest{Text: "browser"})
	require.NoError(t, err)

	assert.Equal(t, "https://api.github.com/search/repositories?q=browser", stub.lastURL(t))
}

// Discovery is anonymous on every host. Quiver holds no credentials, so no
// provider may ever send an Authorization header.
func TestGitHub_Search_NeverSendsAuthorization(t *testing.T) {
	stub := &stubDoer{response: okBody(githubPayload)}

	_, err := newGitHub(t, stub).Search(context.Background(), provider.SearchRequest{Text: "x"})
	require.NoError(t, err)

	assert.Empty(t, stub.lastHeaders(t).Get("Authorization"))
}

// ─── GitLab ──────────────────────────────────────────────────────────────────

func TestGitLab_Search_ParsesCandidates(t *testing.T) {
	stub := &stubDoer{response: okBody(gitlabPayload)}
	p := newProvider(t, metadata.SearchKindGitLab, stub, nil)

	got, err := p.Search(context.Background(), provider.SearchRequest{
		Text:   "browser",
		Topics: []string{"quiver-arrow"},
		Limit:  25,
	})
	require.NoError(t, err)
	require.Len(t, got, 1)

	assert.Equal(t, provider.Candidate{
		Namespace:     domain.Namespace("gitlab.com/acme/chromium"),
		Name:          "chromium",
		Description:   "A fast browser",
		Stars:         42,
		Source:        "gitlab.com",
		DefaultBranch: "master",
	}, got[0])
}

func TestGitLab_Search_SendsTopicAsItsOwnParameter(t *testing.T) {
	stub := &stubDoer{response: okBody(gitlabPayload)}
	p := newProvider(t, metadata.SearchKindGitLab, stub, nil)

	_, err := p.Search(context.Background(), provider.SearchRequest{
		Text:   "web browser",
		Topics: []string{"quiver-arrow"},
		Limit:  25,
	})
	require.NoError(t, err)

	assert.Equal(
		t,
		"https://gitlab.com/api/v4/projects?search=web+browser&topic=quiver-arrow&per_page=25",
		stub.lastURL(t),
	)
}

func TestGitLab_Search_MalformedJSON(t *testing.T) {
	stub := &stubDoer{response: okBody(`[{`)}
	p := newProvider(t, metadata.SearchKindGitLab, stub, nil)

	_, err := p.Search(context.Background(), provider.SearchRequest{Text: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}

func TestGitLab_Search_EmptyResults(t *testing.T) {
	stub := &stubDoer{response: okBody(`[]`)}
	p := newProvider(t, metadata.SearchKindGitLab, stub, nil)

	got, err := p.Search(context.Background(), provider.SearchRequest{Text: "x"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

// A nested group path produces a four-segment namespace, which collides with
// the quiver-hosted form and cannot be resolved, so it is dropped.
func TestGitLab_Search_SkipsNestedGroupPaths(t *testing.T) {
	stub := &stubDoer{response: okBody(`[
		{"path_with_namespace": "group/sub/deep/repo", "name": "repo", "default_branch": "main"},
		{"path_with_namespace": "acme/ok", "name": "ok", "default_branch": "main"}
	]`)}
	p := newProvider(t, metadata.SearchKindGitLab, stub, nil)

	got, err := p.Search(context.Background(), provider.SearchRequest{Text: "x"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, domain.Namespace("gitlab.com/acme/ok"), got[0].Namespace)
}

func TestGitLab_Search_RateLimited429(t *testing.T) {
	headers := http.Header{}
	headers.Set("Retry-After", "12")

	stub := &stubDoer{response: fns.Response{Status: http.StatusTooManyRequests, Headers: headers}}
	p := newProvider(t, metadata.SearchKindGitLab, stub, nil)

	_, err := p.Search(context.Background(), provider.SearchRequest{Text: "x"})

	var limited *provider.RateLimitedError
	require.ErrorAs(t, err, &limited)
	assert.Equal(t, "gitlab.com", limited.Host)
	assert.Equal(t, 12*time.Second, limited.RetryAfter)
}

// ─── topics ──────────────────────────────────────────────────────────────────

// Both hosts intersect multiple markers rather than union them: GitHub ANDs
// repeated topic qualifiers and GitLab ANDs a comma-separated topic list.
func TestProviders_TwoTopicsProduceCorrectQuery(t *testing.T) {
	testCases := []struct {
		name string
		kind string
		want string
	}{
		{
			name: "github repeats the qualifier",
			kind: metadata.SearchKindGitHub,
			want: "https://api.github.com/search/repositories?q=x+topic%3Aquiver-arrow+topic%3Aquiver-beta&per_page=25",
		},
		{
			name: "gitlab sends a comma separated list",
			kind: metadata.SearchKindGitLab,
			want: "https://gitlab.com/api/v4/projects?search=x&topic=quiver-arrow%2Cquiver-beta&per_page=25",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubDoer{response: okBody(`{"items": []}`)}
			if tc.kind == metadata.SearchKindGitLab {
				stub.response = okBody(`[]`)
			}
			p := newProvider(t, tc.kind, stub, nil)

			_, err := p.Search(context.Background(), provider.SearchRequest{
				Text:   "x",
				Topics: []string{"quiver-arrow", "quiver-beta"},
				Limit:  25,
			})
			require.NoError(t, err)
			assert.Equal(t, tc.want, stub.lastURL(t))
		})
	}
}

// ─── construction from platforms ─────────────────────────────────────────────

func TestFromPlatforms_PlatformWithoutSearchURLIsExcluded(t *testing.T) {
	platforms := metadata.Platforms{
		"github.com": {
			SearchURL:  "https://api.github.com/search/repositories?q={query}",
			SearchKind: metadata.SearchKindGitHub,
		},
		"gitlab.com": {
			SearchURL:  "https://gitlab.com/api/v4/projects?search={query}&topic={topic}",
			SearchKind: metadata.SearchKindGitLab,
		},
		"bitbucket.org": {
			RawURL: "https://bitbucket.org/{user}/{repo}/raw/{branch}/{file}",
		},
	}

	providers, err := provider.FromPlatforms(platforms, provider.Config{Timeout: time.Second})
	require.NoError(t, err)

	hosts := make([]string, 0, len(providers))
	for _, p := range providers {
		hosts = append(hosts, p.Host())
	}
	assert.Equal(t, []string{"github.com", "gitlab.com"}, hosts)
}

func TestFromPlatforms_NoSearchablePlatforms_ReturnsEmpty(t *testing.T) {
	providers, err := provider.FromPlatforms(
		metadata.Platforms{"bitbucket.org": {RawURL: "x"}},
		provider.Config{},
	)
	require.NoError(t, err)
	assert.Empty(t, providers)
}

func TestFromPlatforms_UnknownSearchKind_ReturnsError(t *testing.T) {
	_, err := provider.FromPlatforms(
		metadata.Platforms{"example.com": {SearchURL: "https://x?q={query}", SearchKind: "gitea"}},
		provider.Config{},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "example.com")
}

func TestFromPlatforms_CarriesTimeout(t *testing.T) {
	stub := &stubDoer{response: okBody(githubPayload)}

	providers, err := provider.FromPlatforms(
		metadata.Platforms{"github.com": {
			SearchURL:  "https://api.github.com/search/repositories?q={query}",
			SearchKind: metadata.SearchKindGitHub,
		}},
		provider.Config{Timeout: time.Second, Do: stub.do, Now: fixedNow},
	)
	require.NoError(t, err)
	require.Len(t, providers, 1)

	_, err = providers[0].Search(context.Background(), provider.SearchRequest{Text: "x"})
	require.NoError(t, err)
	assert.Empty(t, stub.lastHeaders(t).Get("Authorization"))
}

// ─── timeout and cancellation ────────────────────────────────────────────────

func TestProvider_Search_CancelledContext_ReturnsPromptly(t *testing.T) {
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

	p := newProvider(t, metadata.SearchKindGitHub, &stubDoer{}, func(c *provider.Config) {
		c.Do = blocking
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := p.Search(ctx, provider.SearchRequest{Text: "x"})
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
func TestProvider_Search_AppliesItsOwnTimeout(t *testing.T) {
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

	p := newProvider(t, metadata.SearchKindGitHub, &stubDoer{}, func(c *provider.Config) {
		c.Do = blocking
		c.Timeout = time.Nanosecond
	})

	_, err := p.Search(context.Background(), provider.SearchRequest{Text: "x"})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestProvider_Search_ZeroTimeout_DoesNotBoundTheRequest(t *testing.T) {
	stub := &stubDoer{response: okBody(githubPayload)}
	p := newProvider(t, metadata.SearchKindGitHub, stub, func(c *provider.Config) {
		c.Timeout = 0
	})

	got, err := p.Search(context.Background(), provider.SearchRequest{Text: "x"})
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestProvider_Search_NilDo_UsesTheDefaultTransport(t *testing.T) {
	p, err := provider.New(provider.Config{
		Host:       "github.com",
		SearchURL:  "https://api.github.com/search/repositories?q={query}",
		SearchKind: metadata.SearchKindGitHub,
		Timeout:    time.Nanosecond,
	})
	require.NoError(t, err)

	// The nanosecond deadline expires before any socket is opened, so this
	// exercises the default doer without touching the network.
	_, err = p.Search(context.Background(), provider.SearchRequest{Text: "x"})
	require.Error(t, err)
}

func TestRateLimitedError_Error_WithoutRetryAfter(t *testing.T) {
	err := &provider.RateLimitedError{Host: "github.com"}
	assert.Equal(t, "github.com is rate limited", err.Error())
}

func TestRateLimitedError_Error_WithRetryAfter(t *testing.T) {
	err := &provider.RateLimitedError{Host: "github.com", RetryAfter: 40 * time.Second}
	assert.Equal(t, "github.com is rate limited, retry in 40s", err.Error())
}

func TestGitHub_Search_UnparseableRetryAfter_IsZero(t *testing.T) {
	headers := http.Header{}
	headers.Set("Retry-After", "later")

	stub := &stubDoer{response: fns.Response{Status: http.StatusTooManyRequests, Headers: headers}}

	_, err := newGitHub(t, stub).Search(context.Background(), provider.SearchRequest{Text: "x"})

	var limited *provider.RateLimitedError
	require.ErrorAs(t, err, &limited)
	assert.Zero(t, limited.RetryAfter)
}

func TestGitHub_Search_NegativeRetryAfter_ClampsToZero(t *testing.T) {
	headers := http.Header{}
	headers.Set("Retry-After", "-5")

	stub := &stubDoer{response: fns.Response{Status: http.StatusTooManyRequests, Headers: headers}}

	_, err := newGitHub(t, stub).Search(context.Background(), provider.SearchRequest{Text: "x"})

	var limited *provider.RateLimitedError
	require.ErrorAs(t, err, &limited)
	assert.Zero(t, limited.RetryAfter)
}
