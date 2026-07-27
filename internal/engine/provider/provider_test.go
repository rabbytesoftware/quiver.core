package provider_test

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	"github.com/rabbytesoftware/quiver.core/internal/core/metadata"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
)

// stubDoer answers every request with one canned response and records what it
// was asked for, so a construction test can prove what reached the wire without
// opening a socket.
type stubDoer struct {
	mu       sync.Mutex
	requests []fns.Request
	response fns.Response
}

func (s *stubDoer) do(
	_ context.Context,
	req fns.Request,
) (fns.Response, error) {
	s.mu.Lock()
	s.requests = append(s.requests, req)
	s.mu.Unlock()
	return s.response, nil
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

const githubPayload = `{"items": [
  {"full_name": "acme/chromium", "name": "chromium", "stargazers_count": 42, "default_branch": "master"}
]}`

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

func TestNew_BuildsTheProviderForTheConfiguredKind(t *testing.T) {
	testCases := []struct {
		name string
		kind string
		host string
	}{
		{name: "github", kind: metadata.SearchKindGitHub, host: "github.com"},
		{name: "gitlab", kind: metadata.SearchKindGitLab, host: "gitlab.com"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := provider.New(provider.Config{
				Host:       tc.host,
				SearchURL:  "https://" + tc.host + "/search?q={query}&topic={topic}",
				SearchKind: tc.kind,
			})
			require.NoError(t, err)
			assert.Equal(t, tc.host, p.Host())
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

func TestFromPlatforms_CarriesTheBaseConfig(t *testing.T) {
	stub := &stubDoer{response: okBody(githubPayload)}

	providers, err := provider.FromPlatforms(
		metadata.Platforms{"github.com": {
			SearchURL:  "https://api.github.com/search/repositories?q={query}",
			SearchKind: metadata.SearchKindGitHub,
		}},
		provider.Config{Timeout: time.Second, Do: stub.do, Now: time.Now},
	)
	require.NoError(t, err)
	require.Len(t, providers, 1)

	got, err := providers[0].Search(context.Background(), provider.SearchRequest{Text: "x"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Empty(t, stub.lastHeaders(t).Get("Authorization"))
}
