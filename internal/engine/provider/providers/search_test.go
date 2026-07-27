package providers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// A marker list is a union: adding a marker must widen the result set. Both
// hosts natively intersect a multi-topic query, so each marker gets its own
// request and the provider unions the responses.
func TestSearchEachTopic_EveryTopicGetsItsOwnRequest(t *testing.T) {
	testCases := []struct {
		name  string
		build func(*stubDoer) Provider
		body  string
		want  []string
	}{
		{
			name:  "github",
			build: newGitHub,
			body:  `{"items": []}`,
			want: []string{
				"https://api.github.com/search/repositories?q=x+topic%3Aquiver-arrow&per_page=25",
				"https://api.github.com/search/repositories?q=x+topic%3Aquiver-beta&per_page=25",
			},
		},
		{
			name:  "gitlab",
			build: newGitLab,
			body:  `[]`,
			want: []string{
				"https://gitlab.com/api/v4/projects?search=x&topic=quiver-arrow&per_page=25",
				"https://gitlab.com/api/v4/projects?search=x&topic=quiver-beta&per_page=25",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubDoer{response: okBody(tc.body)}

			_, err := tc.build(stub).Search(context.Background(), SearchRequest{
				Text:   "x",
				Topics: []string{"quiver-arrow", "quiver-beta"},
				Limit:  25,
			})
			require.NoError(t, err)
			assert.Equal(t, tc.want, stub.urls(t),
				"one metered request per marker, in order")
		})
	}
}

// A failure on any marker fails the whole search: a partial union would look
// like a smaller ecosystem rather than a broken request.
func TestSearchEachTopic_TopicRequestFailureAbortsTheUnion(t *testing.T) {
	stub := &stubDoer{err: errors.New("network unreachable")}

	_, err := newGitHub(stub).Search(context.Background(), SearchRequest{
		Text:   "x",
		Topics: []string{"quiver-arrow", "quiver-beta"},
		Limit:  25,
	})

	require.Error(t, err)
	assert.Len(t, stub.urls(t), 1, "the second marker is not attempted after the first fails")
}

// A repo carrying both markers must appear once, not once per marker.
func TestSearchEachTopic_UnionDedupesAcrossTopics(t *testing.T) {
	body := `{"items": [{"full_name": "acme/thing", "name": "thing", "default_branch": "main"}]}`
	stub := &stubDoer{response: okBody(body)}

	got, err := newGitHub(stub).Search(context.Background(), SearchRequest{
		Text:   "thing",
		Topics: []string{"quiver-arrow", "quiver-beta"},
		Limit:  25,
	})

	require.NoError(t, err)
	require.Len(t, got, 1, "the same repo under two markers is one candidate")
	assert.Equal(t, domain.Namespace("github.com/acme/thing"), got[0].Namespace)
	assert.Len(t, stub.urls(t), 2, "but it still cost two requests")
}

// An empty marker list must still search, with no topic filter and one request.
func TestSearchEachTopic_NoTopicsIssuesOneUnfilteredRequest(t *testing.T) {
	stub := &stubDoer{response: okBody(`{"items": []}`)}

	_, err := newGitHub(stub).Search(context.Background(), SearchRequest{Text: "x", Limit: 25})

	require.NoError(t, err)
	assert.Equal(t, []string{
		"https://api.github.com/search/repositories?q=x&per_page=25",
	}, stub.urls(t))
}

// The union is capped by the same limit one request is, so two markers cannot
// return twice the page the caller asked for.
func TestSearchEachTopic_UnionTruncatesToTheRequestedLimit(t *testing.T) {
	stub := &stubDoer{response: okBody(githubPayload)}

	got, err := newGitHub(stub).Search(context.Background(), SearchRequest{
		Text:   "browser",
		Topics: []string{"quiver-arrow"},
		Limit:  1,
	})

	require.NoError(t, err)
	assert.Len(t, got, 1)
}
