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

func redirectResponse(
	location string,
) fns.Response {
	headers := http.Header{}
	headers.Set("Location", location)
	return fns.Response{Status: http.StatusFound, Headers: headers}
}

func githubForgeConfig(
	doer *routedDoer,
) Config {
	return Config{
		Host:              "github.com",
		RawURL:            "https://raw.githubusercontent.com/{user}/{repo}/{branch}/{file}",
		ExpandedAssetsURL: "https://github.com/{user}/{repo}/releases/expanded_assets/{tag}",
		RepoPageURL:       "https://github.com/{user}/{repo}",
		OrgURL:            "https://github.com/orgs/{user}",
		AvatarURL:         "https://github.com/{user}.png",
		Timeout:           5 * time.Second,
		Do:                doer.do,
	}
}

func TestGitHub_ImplementsForge(t *testing.T) {
	p := NewGitHub(githubForgeConfig(&routedDoer{}))
	_, ok := p.(Forge)
	assert.True(t, ok)
}

func TestGitHub_RawFile_200_ReturnsBody(t *testing.T) {
	doer := &routedDoer{responses: map[string]fns.Response{
		"https://raw.githubusercontent.com/u/r/main/ARROW.md": okBody("name: r\n"),
	}}
	forge := NewGitHub(githubForgeConfig(doer)).(Forge)

	body, err := forge.RawFile(context.Background(), domain.Namespace("github.com/u/r"), "main", "ARROW.md")
	require.NoError(t, err)
	assert.Equal(t, "name: r\n", string(body))
}

func TestGitHub_RawFile_404_ReturnsErrRawNotFound(t *testing.T) {
	forge := NewGitHub(githubForgeConfig(&routedDoer{})).(Forge)

	_, err := forge.RawFile(context.Background(), domain.Namespace("github.com/u/r"), "main", "ARROW.md")
	assert.ErrorIs(t, err, ErrRawNotFound)
}

func TestGitHub_RawFile_TransportFailure_ReturnsError(t *testing.T) {
	doer := &routedDoer{failures: map[string]error{
		"https://raw.githubusercontent.com/u/r/main/ARROW.md": errors.New("dial tcp: connection refused"),
	}}
	forge := NewGitHub(githubForgeConfig(doer)).(Forge)

	_, err := forge.RawFile(context.Background(), domain.Namespace("github.com/u/r"), "main", "ARROW.md")
	require.Error(t, err)
}

func TestGitHub_RawFile_InvalidNamespace_ReturnsError(t *testing.T) {
	forge := NewGitHub(githubForgeConfig(&routedDoer{})).(Forge)

	_, err := forge.RawFile(context.Background(), domain.Namespace("only-two"), "main", "ARROW.md")
	require.Error(t, err)
}

func TestGitHub_RawFile_ServerError_ReturnsAGenericError(t *testing.T) {
	doer := &routedDoer{responses: map[string]fns.Response{
		"https://raw.githubusercontent.com/u/r/main/ARROW.md": {Status: http.StatusInternalServerError, Headers: http.Header{}},
	}}
	forge := NewGitHub(githubForgeConfig(doer)).(Forge)

	_, err := forge.RawFile(context.Background(), domain.Namespace("github.com/u/r"), "main", "ARROW.md")
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrRawNotFound))
}

func TestGitHub_ReleaseAssets_UsesTheExpandedAssetsURLTemplate(t *testing.T) {
	doer := &routedDoer{responses: map[string]fns.Response{}}
	forge := NewGitHub(githubForgeConfig(doer)).(Forge)

	_, _ = forge.ReleaseAssets(context.Background(), domain.Namespace("github.com/u/r"), "v1.0.0")

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
			forge := NewGitHub(githubForgeConfig(doer)).(Forge)

			_, _ = forge.ReleaseAssets(context.Background(), domain.Namespace("github.com/u/r"), tc.tag)

			require.Len(t, doer.requests, 1)
			assert.Equal(t, tc.want, doer.requests[0])
		})
	}
}

func TestGitHub_ReleaseAssets_TransportFailure_ReturnsError(t *testing.T) {
	doer := &routedDoer{failures: map[string]error{
		"https://github.com/u/r/releases/expanded_assets/v1": errors.New("dial tcp: connection refused"),
	}}
	forge := NewGitHub(githubForgeConfig(doer)).(Forge)

	_, err := forge.ReleaseAssets(context.Background(), domain.Namespace("github.com/u/r"), "v1")
	require.Error(t, err)
}

func TestGitHub_ReleaseAssets_InvalidNamespace_ReturnsError(t *testing.T) {
	forge := NewGitHub(githubForgeConfig(&routedDoer{})).(Forge)

	_, err := forge.ReleaseAssets(context.Background(), domain.Namespace("only-two"), "v1")
	require.Error(t, err)
}

func TestGitHub_ReleaseAssets_NoExpandedAssetsURLConfigured_ReturnsErrNoRawURL(t *testing.T) {
	forge := NewGitHub(Config{Host: "github.com", Do: (&routedDoer{}).do}).(Forge)

	_, err := forge.ReleaseAssets(context.Background(), domain.Namespace("github.com/u/r"), "v1")
	assert.ErrorIs(t, err, ErrNoRawURL)
}

func TestGitHub_ReleaseAssets_404_ReturnsErrReleaseNotFound(t *testing.T) {
	forge := NewGitHub(githubForgeConfig(&routedDoer{})).(Forge)

	_, err := forge.ReleaseAssets(context.Background(), domain.Namespace("github.com/u/r"), "missing")
	assert.ErrorIs(t, err, ErrReleaseNotFound)
}

func TestGitHub_ReleaseAssets_ServerError_ReturnsAGenericError(t *testing.T) {
	doer := &routedDoer{responses: map[string]fns.Response{
		"https://github.com/u/r/releases/expanded_assets/v1": {Status: http.StatusInternalServerError, Headers: http.Header{}},
	}}
	forge := NewGitHub(githubForgeConfig(doer)).(Forge)

	_, err := forge.ReleaseAssets(context.Background(), domain.Namespace("github.com/u/r"), "v1")
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrReleaseNotFound))
}

func TestGitHub_ReleaseAssets_GoldenFragment_ParsesAssets(t *testing.T) {
	body := readTestdata(t, "expanded_assets_ripgrep.html")
	doer := &routedDoer{responses: map[string]fns.Response{
		"https://github.com/BurntSushi/ripgrep/releases/expanded_assets/15.2.0": okBody(string(body)),
	}}
	forge := NewGitHub(githubForgeConfig(doer)).(Forge)

	assets, err := forge.ReleaseAssets(context.Background(), domain.Namespace("github.com/BurntSushi/ripgrep"), "15.2.0")
	require.NoError(t, err)
	assert.Len(t, assets, 4)
}

func TestGitHub_ReleaseAssets_MalformedFragment_ReturnsErrUnexpectedPage(t *testing.T) {
	doer := &routedDoer{responses: map[string]fns.Response{
		"https://github.com/u/r/releases/expanded_assets/v1": okBody("<html><body>oops</body></html>"),
	}}
	forge := NewGitHub(githubForgeConfig(doer)).(Forge)

	_, err := forge.ReleaseAssets(context.Background(), domain.Namespace("github.com/u/r"), "v1")
	assert.ErrorIs(t, err, ErrUnexpectedPage)
}

func TestGitHub_RepoPage_UserOwner_ParsesMetadataAndOwnerKind(t *testing.T) {
	pageBody := readTestdata(t, "repo_page_ripgrep.html")
	doer := &routedDoer{responses: map[string]fns.Response{
		"https://github.com/BurntSushi/ripgrep": okBody(string(pageBody)),
		"https://github.com/orgs/BurntSushi":    {Status: http.StatusNotFound, Headers: http.Header{}},
		"https://github.com/BurntSushi.png":     redirectResponse("https://avatars.githubusercontent.com/u/1234?v=4"),
	}}
	forge := NewGitHub(githubForgeConfig(doer)).(Forge)

	page, err := forge.RepoPage(context.Background(), domain.Namespace("github.com/BurntSushi/ripgrep"))
	require.NoError(t, err)
	assert.Contains(t, page.Description, "ripgrep recursively searches")
	assert.False(t, page.OwnerIsOrg)
	assert.Equal(t, "https://avatars.githubusercontent.com/u/1234?v=4", page.OwnerAvatar)
}

func TestGitHub_RepoPage_OrgOwner_ReturnsOwnerIsOrgTrue(t *testing.T) {
	pageBody := readTestdata(t, "repo_page_ripgrep.html")
	doer := &routedDoer{responses: map[string]fns.Response{
		"https://github.com/BurntSushi/ripgrep": okBody(string(pageBody)),
		"https://github.com/orgs/BurntSushi":    redirectResponse("https://github.com/BurntSushi"),
		"https://github.com/BurntSushi.png":     redirectResponse("https://avatars.githubusercontent.com/u/1234?v=4"),
	}}
	forge := NewGitHub(githubForgeConfig(doer)).(Forge)

	page, err := forge.RepoPage(context.Background(), domain.Namespace("github.com/BurntSushi/ripgrep"))
	require.NoError(t, err)
	assert.True(t, page.OwnerIsOrg)
}

func TestGitHub_RepoPage_InvalidNamespace_ReturnsError(t *testing.T) {
	forge := NewGitHub(githubForgeConfig(&routedDoer{})).(Forge)

	_, err := forge.RepoPage(context.Background(), domain.Namespace("only-two"))
	require.Error(t, err)
}

func TestGitHub_RepoPage_NoRepoPageURLConfigured_ReturnsErrNoRawURL(t *testing.T) {
	forge := NewGitHub(Config{Host: "github.com", Do: (&routedDoer{}).do}).(Forge)

	_, err := forge.RepoPage(context.Background(), domain.Namespace("github.com/u/r"))
	assert.ErrorIs(t, err, ErrNoRawURL)
}

func TestGitHub_RepoPage_TransportFailure_ReturnsError(t *testing.T) {
	doer := &routedDoer{failures: map[string]error{
		"https://github.com/u/r": errors.New("dial tcp: connection refused"),
	}}
	forge := NewGitHub(githubForgeConfig(doer)).(Forge)

	_, err := forge.RepoPage(context.Background(), domain.Namespace("github.com/u/r"))
	require.Error(t, err)
}

func TestGitHub_RepoPage_NoOrgURLConfigured_SkipsTheOwnerCheck(t *testing.T) {
	pageBody := readTestdata(t, "repo_page_ripgrep.html")
	doer := &routedDoer{responses: map[string]fns.Response{
		"https://github.com/BurntSushi/ripgrep": okBody(string(pageBody)),
	}}
	forge := NewGitHub(Config{
		Host:        "github.com",
		RepoPageURL: "https://github.com/{user}/{repo}",
		Do:          doer.do,
	}).(Forge)

	page, err := forge.RepoPage(context.Background(), domain.Namespace("github.com/BurntSushi/ripgrep"))
	require.NoError(t, err)
	assert.False(t, page.OwnerIsOrg)
	assert.Empty(t, page.OwnerAvatar)
}

func TestGitHub_RepoPage_404_ReturnsError(t *testing.T) {
	forge := NewGitHub(githubForgeConfig(&routedDoer{})).(Forge)

	_, err := forge.RepoPage(context.Background(), domain.Namespace("github.com/u/r"))
	require.Error(t, err)
}

func TestGitHub_RepoPage_MalformedPage_ReturnsErrUnexpectedPage(t *testing.T) {
	doer := &routedDoer{responses: map[string]fns.Response{
		"https://github.com/u/r": okBody("<html><body>oops</body></html>"),
	}}
	forge := NewGitHub(githubForgeConfig(doer)).(Forge)

	_, err := forge.RepoPage(context.Background(), domain.Namespace("github.com/u/r"))
	assert.ErrorIs(t, err, ErrUnexpectedPage)
}

func TestGitHub_RepoPage_OrgCheckPlainOK_ReturnsOwnerIsOrgTrue(t *testing.T) {
	pageBody := readTestdata(t, "repo_page_ripgrep.html")
	doer := &routedDoer{responses: map[string]fns.Response{
		"https://github.com/BurntSushi/ripgrep": okBody(string(pageBody)),
		"https://github.com/orgs/BurntSushi":    {Status: http.StatusOK, Headers: http.Header{}},
		"https://github.com/BurntSushi.png":     redirectResponse("https://avatars.githubusercontent.com/u/1234?v=4"),
	}}
	forge := NewGitHub(githubForgeConfig(doer)).(Forge)

	page, err := forge.RepoPage(context.Background(), domain.Namespace("github.com/BurntSushi/ripgrep"))
	require.NoError(t, err)
	assert.True(t, page.OwnerIsOrg)
}

func TestGitHub_RepoPage_OrgCheckUnexpectedStatus_ReturnsError(t *testing.T) {
	pageBody := readTestdata(t, "repo_page_ripgrep.html")
	doer := &routedDoer{responses: map[string]fns.Response{
		"https://github.com/BurntSushi/ripgrep": okBody(string(pageBody)),
		"https://github.com/orgs/BurntSushi":    {Status: http.StatusInternalServerError, Headers: http.Header{}},
	}}
	forge := NewGitHub(githubForgeConfig(doer)).(Forge)

	_, err := forge.RepoPage(context.Background(), domain.Namespace("github.com/BurntSushi/ripgrep"))
	require.Error(t, err)
}

func TestGitHub_RepoPage_OrgCheckTransportFailure_ReturnsError(t *testing.T) {
	pageBody := readTestdata(t, "repo_page_ripgrep.html")
	doer := &routedDoer{
		responses: map[string]fns.Response{
			"https://github.com/BurntSushi/ripgrep": okBody(string(pageBody)),
		},
		failures: map[string]error{
			"https://github.com/orgs/BurntSushi": errors.New("dial tcp: connection refused"),
		},
	}
	forge := NewGitHub(githubForgeConfig(doer)).(Forge)

	_, err := forge.RepoPage(context.Background(), domain.Namespace("github.com/BurntSushi/ripgrep"))
	require.Error(t, err)
}

func TestGitHub_RepoPage_AvatarRedirectMissingLocation_FallsBackToTheTemplateURL(t *testing.T) {
	pageBody := readTestdata(t, "repo_page_ripgrep.html")
	doer := &routedDoer{responses: map[string]fns.Response{
		"https://github.com/BurntSushi/ripgrep": okBody(string(pageBody)),
		"https://github.com/orgs/BurntSushi":    {Status: http.StatusNotFound, Headers: http.Header{}},
	}}
	forge := NewGitHub(githubForgeConfig(doer)).(Forge)

	page, err := forge.RepoPage(context.Background(), domain.Namespace("github.com/BurntSushi/ripgrep"))
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/BurntSushi.png", page.OwnerAvatar)
}

func TestGitHub_RepoPage_AvatarRedirectTransportFailure_FallsBackToTheTemplateURL(t *testing.T) {
	pageBody := readTestdata(t, "repo_page_ripgrep.html")
	doer := &routedDoer{
		responses: map[string]fns.Response{
			"https://github.com/BurntSushi/ripgrep": okBody(string(pageBody)),
			"https://github.com/orgs/BurntSushi":    {Status: http.StatusNotFound, Headers: http.Header{}},
		},
		failures: map[string]error{
			"https://github.com/BurntSushi.png": errors.New("dial tcp: connection refused"),
		},
	}
	forge := NewGitHub(githubForgeConfig(doer)).(Forge)

	page, err := forge.RepoPage(context.Background(), domain.Namespace("github.com/BurntSushi/ripgrep"))
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/BurntSushi.png", page.OwnerAvatar)
}
