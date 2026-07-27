package providers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestGitHub_Host_ReturnsConfiguredHost(t *testing.T) {
	assert.Equal(t, "github.com", newGitHub(&stubDoer{response: okBody(githubPayload)}).Host())
}

func TestGitHub_Search_ParsesCandidates(t *testing.T) {
	stub := &stubDoer{response: okBody(githubPayload)}

	got, err := newGitHub(stub).Search(context.Background(), SearchRequest{
		Text:   "browser",
		Topics: []string{"quiver-arrow"},
		Limit:  25,
	})
	require.NoError(t, err)
	require.Len(t, got, 2)

	assert.Equal(t, Candidate{
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

	got, err := newGitHub(stub).Search(context.Background(), SearchRequest{Text: "nothing"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestGitHub_Search_MalformedJSON(t *testing.T) {
	stub := &stubDoer{response: okBody(`{"items": [`)}

	_, err := newGitHub(stub).Search(context.Background(), SearchRequest{Text: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}

func TestGitHub_Search_SkipsRepositoriesWithAnUnusableFullName(t *testing.T) {
	stub := &stubDoer{response: okBody(`{"items": [
		{"full_name": "no-slash", "name": "x", "default_branch": "main"},
		{"full_name": "acme/ok", "name": "ok", "default_branch": "main"}
	]}`)}

	got, err := newGitHub(stub).Search(context.Background(), SearchRequest{Text: "x"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, domain.Namespace("github.com/acme/ok"), got[0].Namespace)
}

func TestGitHub_Search_TruncatesToTheRequestedLimit(t *testing.T) {
	stub := &stubDoer{response: okBody(githubPayload)}

	got, err := newGitHub(stub).Search(context.Background(), SearchRequest{Text: "browser", Limit: 1})
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestGitHub_Search_SendsTopicInQuery(t *testing.T) {
	stub := &stubDoer{response: okBody(githubPayload)}

	_, err := newGitHub(stub).Search(context.Background(), SearchRequest{
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

	_, err := newGitHub(stub).Search(context.Background(), SearchRequest{Text: "browser", Limit: 5})
	require.NoError(t, err)

	assert.Equal(
		t,
		"https://api.github.com/search/repositories?q=browser&per_page=5",
		stub.lastURL(t),
	)
}

func TestGitHub_Search_ZeroLimit_OmitsPerPage(t *testing.T) {
	stub := &stubDoer{response: okBody(githubPayload)}

	_, err := newGitHub(stub).Search(context.Background(), SearchRequest{Text: "browser"})
	require.NoError(t, err)

	assert.Equal(t, "https://api.github.com/search/repositories?q=browser", stub.lastURL(t))
}

// Discovery is anonymous on every host. Quiver holds no credentials, so no
// provider may ever send an Authorization header.
func TestGitHub_Search_NeverSendsAuthorization(t *testing.T) {
	stub := &stubDoer{response: okBody(githubPayload)}

	_, err := newGitHub(stub).Search(context.Background(), SearchRequest{Text: "x"})
	require.NoError(t, err)

	assert.Empty(t, stub.lastHeaders(t).Get("Authorization"))
}
