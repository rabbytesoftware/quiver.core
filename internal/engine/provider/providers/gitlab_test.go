package providers

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestGitLab_Host_ReturnsConfiguredHost(t *testing.T) {
	assert.Equal(t, "gitlab.com", newGitLab(&stubDoer{response: okBody(gitlabPayload)}).Host())
}

func TestGitLab_Search_ParsesCandidates(t *testing.T) {
	stub := &stubDoer{response: okBody(gitlabPayload)}

	got, err := newGitLab(stub).Search(context.Background(), SearchRequest{
		Text:   "browser",
		Topics: []string{"quiver-arrow"},
		Limit:  25,
	})
	require.NoError(t, err)
	require.Len(t, got, 1)

	assert.Equal(t, Candidate{
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

	_, err := newGitLab(stub).Search(context.Background(), SearchRequest{
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

	_, err := newGitLab(stub).Search(context.Background(), SearchRequest{Text: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode")
}

func TestGitLab_Search_EmptyResults(t *testing.T) {
	stub := &stubDoer{response: okBody(`[]`)}

	got, err := newGitLab(stub).Search(context.Background(), SearchRequest{Text: "x"})
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

	got, err := newGitLab(stub).Search(context.Background(), SearchRequest{Text: "x"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, domain.Namespace("gitlab.com/acme/ok"), got[0].Namespace)
}

func TestGitLab_Search_RateLimited429(t *testing.T) {
	headers := http.Header{}
	headers.Set("Retry-After", "12")

	stub := &stubDoer{response: fns.Response{Status: http.StatusTooManyRequests, Headers: headers}}

	_, err := newGitLab(stub).Search(context.Background(), SearchRequest{Text: "x"})

	var limited *RateLimitedError
	require.ErrorAs(t, err, &limited)
	assert.Equal(t, "gitlab.com", limited.Host)
	assert.Equal(t, 12*time.Second, limited.RetryAfter)
}

func TestGitLab_Search_NeverSendsAuthorization(t *testing.T) {
	stub := &stubDoer{response: okBody(gitlabPayload)}

	_, err := newGitLab(stub).Search(context.Background(), SearchRequest{Text: "x"})
	require.NoError(t, err)

	assert.Empty(t, stub.lastHeaders(t).Get("Authorization"))
}
