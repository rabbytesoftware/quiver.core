package providers

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type routedDoer struct {
	mu        sync.Mutex
	responses map[string]fns.Response
	failures  map[string]error
	requests  []string
}

func (r *routedDoer) do(
	_ context.Context,
	req fns.Request,
) (fns.Response, error) {
	r.mu.Lock()
	r.requests = append(r.requests, req.URL)
	r.mu.Unlock()

	if err, ok := r.failures[req.URL]; ok {
		return fns.Response{}, err
	}
	resp, ok := r.responses[req.URL]
	if !ok {
		return fns.Response{Status: http.StatusNotFound, Headers: http.Header{}}, nil
	}
	return resp, nil
}

func githubReleaseConfig(
	doer *routedDoer,
) Config {
	return Config{
		Host:              "github.com",
		RawURL:            "https://raw.githubusercontent.com/{user}/{repo}/{branch}/{file}",
		ExpandedAssetsURL: "https://github.com/{user}/{repo}/releases/expanded_assets/{tag}",
		RepoPageURL:       "https://github.com/{user}/{repo}",
		Timeout:           5 * time.Second,
		Do:                doer.do,
	}
}

func TestGitHub_ReleaseAssets_UsesTheExpandedAssetsURLTemplate(t *testing.T) {
	doer := &routedDoer{responses: map[string]fns.Response{}}
	provider := NewGitHub(githubReleaseConfig(doer))

	_, _ = provider.ReleaseAssets(context.Background(), domain.Namespace("github.com/u/r"), "v1.0.0")

	require.Len(t, doer.requests, 1)
	assert.Equal(t, "https://github.com/u/r/releases/expanded_assets/v1.0.0", doer.requests[0])
}

func TestGitHub_ReleaseAssets_EscapesTheTagInTheURL(t *testing.T) {
	testCases := []struct {
		name string
		tag  string
		want string
	}{
		{
			name: "plus sign",
			tag:  "v1.0+build",
			want: "https://github.com/u/r/releases/expanded_assets/v1.0+build",
		},
		{
			name: "slash",
			tag:  "release/2026",
			want: "https://github.com/u/r/releases/expanded_assets/release%2F2026",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doer := &routedDoer{responses: map[string]fns.Response{}}
			provider := NewGitHub(githubReleaseConfig(doer))

			_, _ = provider.ReleaseAssets(context.Background(), domain.Namespace("github.com/u/r"), tc.tag)

			require.Len(t, doer.requests, 1)
			assert.Equal(t, tc.want, doer.requests[0])
		})
	}
}

func TestGitHub_ReleaseAssets_Responses(t *testing.T) {
	const page = "https://github.com/u/r/releases/expanded_assets/v1"

	testCases := []struct {
		name    string
		doer    *routedDoer
		wantErr bool
	}{
		{name: "not found is no assets", doer: &routedDoer{}},
		{
			name: "server error",
			doer: &routedDoer{responses: map[string]fns.Response{
				page: {Status: http.StatusInternalServerError, Headers: http.Header{}},
			}},
			wantErr: true,
		},
		{
			name:    "transport failure",
			doer:    &routedDoer{failures: map[string]error{page: errors.New("dial tcp: connection refused")}},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			provider := NewGitHub(githubReleaseConfig(tc.doer))

			assets, err := provider.ReleaseAssets(context.Background(), domain.Namespace("github.com/u/r"), "v1")

			assert.Equal(t, tc.wantErr, err != nil)
			assert.Empty(t, assets)
		})
	}
}

func TestGitHub_ReleaseAssets_InvalidNamespace_ReturnsError(t *testing.T) {
	provider := NewGitHub(githubReleaseConfig(&routedDoer{}))

	_, err := provider.ReleaseAssets(context.Background(), domain.Namespace("only-two"), "v1")
	require.Error(t, err)
}

func TestGitHub_ReleaseAssets_NoExpandedAssetsURLConfigured_ReturnsErrNoRawURL(t *testing.T) {
	provider := NewGitHub(Config{Host: "github.com", Do: (&routedDoer{}).do})

	_, err := provider.ReleaseAssets(context.Background(), domain.Namespace("github.com/u/r"), "v1")
	assert.ErrorIs(t, err, ErrNoRawURL)
}

func TestGitHub_ReleaseAssets_GoldenFragment_ParsesAssets(t *testing.T) {
	body := readTestdata(t, "expanded_assets_ripgrep.html")
	doer := &routedDoer{responses: map[string]fns.Response{
		"https://github.com/BurntSushi/ripgrep/releases/expanded_assets/15.2.0": okBody(string(body)),
	}}
	provider := NewGitHub(githubReleaseConfig(doer))

	assets, err := provider.ReleaseAssets(context.Background(), domain.Namespace("github.com/BurntSushi/ripgrep"), "15.2.0")
	require.NoError(t, err)
	assert.Len(t, assets, 4)
}

func TestGitHub_ReleaseAssets_MalformedFragment_ReturnsErrUnexpectedPage(t *testing.T) {
	doer := &routedDoer{responses: map[string]fns.Response{
		"https://github.com/u/r/releases/expanded_assets/v1": okBody("<html><body>oops</body></html>"),
	}}
	provider := NewGitHub(githubReleaseConfig(doer))

	_, err := provider.ReleaseAssets(context.Background(), domain.Namespace("github.com/u/r"), "v1")
	assert.ErrorIs(t, err, ErrUnexpectedPage)
}
