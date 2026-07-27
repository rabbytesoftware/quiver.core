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
	"github.com/rabbytesoftware/quiver.core/internal/domain"
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

// platforms is the real platform table's shape, so the construction tests build
// the provider set the daemon builds.
func platforms() metadata.Platforms {
	return metadata.Platforms{
		"github.com": {
			Kind:             metadata.KindGitHub,
			RawURL:           "https://raw.githubusercontent.com/{user}/{repo}/{branch}/{file}",
			DefaultBranches:  []string{"main", "master"},
			LatestReleaseURL: "https://github.com/{user}/{repo}/releases/latest",
			SearchURL:        "https://api.github.com/search/repositories?q={query}",
		},
		"gitlab.com": {
			Kind:             metadata.KindGitLab,
			RawURL:           "https://gitlab.com/{user}/{repo}/-/raw/{branch}/{file}",
			DefaultBranches:  []string{"main", "master"},
			LatestReleaseURL: "https://gitlab.com/{user}/{repo}/-/releases/permalink/latest",
			SearchURL:        "https://gitlab.com/api/v4/projects?search={query}&topic={topic}",
		},
		"bitbucket.org": {
			Kind:            metadata.KindBitbucket,
			RawURL:          "https://bitbucket.org/{user}/{repo}/raw/{branch}/{file}",
			DefaultBranches: []string{"main", "master"},
		},
	}
}

func byHost(providers []provider.Provider) map[string]provider.Provider {
	found := make(map[string]provider.Provider, len(providers))
	for _, p := range providers {
		found[p.Host()] = p
	}
	return found
}

// ─── construction ────────────────────────────────────────────────────────────

func TestNew_UnknownKind_ReturnsError(t *testing.T) {
	_, err := provider.New(provider.Config{
		Host:      "example.com",
		Kind:      "gitea",
		SearchURL: "https://example.com?q={query}",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown kind")
}

func TestNew_EmptyHost_ReturnsError(t *testing.T) {
	_, err := provider.New(provider.Config{Kind: metadata.KindGitHub})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no host")
}

// A host that declares no search endpoint is still a provider: search is a
// capability it lacks, not a reason to have no provider at all.
func TestNew_NoSearchURL_BuildsAProviderThatCannotSearch(t *testing.T) {
	p, err := provider.New(provider.Config{
		Host:   "github.com",
		Kind:   metadata.KindGitHub,
		RawURL: "https://raw.githubusercontent.com/{user}/{repo}/{branch}/{file}",
	})
	require.NoError(t, err)
	assert.False(t, p.CanSearch())

	_, err = p.Search(context.Background(), provider.SearchRequest{Text: "x"})
	assert.ErrorIs(t, err, provider.ErrSearchUnsupported)
}

func TestNew_BuildsTheProviderForTheConfiguredKind(t *testing.T) {
	testCases := []struct {
		name string
		kind string
		host string
	}{
		{name: "github", kind: metadata.KindGitHub, host: "github.com"},
		{name: "gitlab", kind: metadata.KindGitLab, host: "gitlab.com"},
		{name: "bitbucket", kind: metadata.KindBitbucket, host: "bitbucket.org"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := provider.New(provider.Config{Host: tc.host, Kind: tc.kind})
			require.NoError(t, err)
			assert.Equal(t, tc.host, p.Host())
		})
	}
}

// ─── construction from platforms ─────────────────────────────────────────────

// Every platform is a provider. Dropping the ones that cannot search would take
// their raw-file URLs with them, and their manifests would stop resolving.
func TestFromPlatforms_BuildsOneProviderPerPlatform(t *testing.T) {
	providers, err := provider.FromPlatforms(platforms(), provider.Config{Timeout: time.Second})
	require.NoError(t, err)

	hosts := make([]string, 0, len(providers))
	for _, p := range providers {
		hosts = append(hosts, p.Host())
	}
	assert.Equal(t, []string{"bitbucket.org", "github.com", "gitlab.com"}, hosts)
}

func TestFromPlatforms_SearchIsACapabilityNotAnEntryRequirement(t *testing.T) {
	providers, err := provider.FromPlatforms(platforms(), provider.Config{Timeout: time.Second})
	require.NoError(t, err)

	found := byHost(providers)
	assert.True(t, found["github.com"].CanSearch())
	assert.True(t, found["gitlab.com"].CanSearch())
	assert.False(t, found["bitbucket.org"].CanSearch(), "bitbucket exposes no repository search")
}

// Bitbucket is the reason every platform is a provider: it answers no query but
// serves raw files, and a manifest there must stay fetchable.
func TestFromPlatforms_BitbucketServesRawFilesWithoutSearching(t *testing.T) {
	providers, err := provider.FromPlatforms(platforms(), provider.Config{Timeout: time.Second})
	require.NoError(t, err)

	bitbucket := byHost(providers)["bitbucket.org"]
	require.NotNil(t, bitbucket)

	got, err := bitbucket.RawFileURL(domain.Namespace("bitbucket.org/acme/tool"), "main", "ARROW.md")
	require.NoError(t, err)
	assert.Equal(t, "https://bitbucket.org/acme/tool/raw/main/ARROW.md", got)
	assert.Equal(t, []string{"main", "master"}, bitbucket.DefaultBranches())
}

func TestFromPlatforms_EmptyTable_ReturnsEmpty(t *testing.T) {
	providers, err := provider.FromPlatforms(metadata.Platforms{}, provider.Config{})
	require.NoError(t, err)
	assert.Empty(t, providers)
}

func TestFromPlatforms_UnknownKind_ReturnsError(t *testing.T) {
	_, err := provider.FromPlatforms(
		metadata.Platforms{"example.com": {Kind: "gitea", SearchURL: "https://x?q={query}"}},
		provider.Config{},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "example.com")
}

func TestFromPlatforms_CarriesTheBaseConfig(t *testing.T) {
	stub := &stubDoer{response: okBody(githubPayload)}

	providers, err := provider.FromPlatforms(
		metadata.Platforms{"github.com": platforms()["github.com"]},
		provider.Config{Timeout: time.Second, Do: stub.do, Now: time.Now},
	)
	require.NoError(t, err)
	require.Len(t, providers, 1)

	got, err := providers[0].Search(context.Background(), provider.SearchRequest{Text: "x"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Empty(t, stub.lastHeaders(t).Get("Authorization"))
}
