package providers

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/fns"
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

// urls returns every request the stub saw, in order — the assertion surface
// for how many metered calls a search actually cost.
func (s *stubDoer) urls(t *testing.T) []string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.requests))
	for _, req := range s.requests {
		out = append(out, req.URL)
	}
	return out
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

// ─── canned host configs ─────────────────────────────────────────────────────

func githubConfig(stub *stubDoer) Config {
	return Config{
		Host:      "github.com",
		SearchURL: "https://api.github.com/search/repositories?q={query}",
		Timeout:   5 * time.Second,
		Do:        stub.do,
		Now:       fixedNow,
	}
}

func gitlabConfig(stub *stubDoer) Config {
	return Config{
		Host:      "gitlab.com",
		SearchURL: "https://gitlab.com/api/v4/projects?search={query}&topic={topic}",
		Timeout:   5 * time.Second,
		Do:        stub.do,
		Now:       fixedNow,
	}
}

func newGitHub(stub *stubDoer) Provider {
	return NewGitHub(githubConfig(stub))
}

func newGitLab(stub *stubDoer) Provider {
	return NewGitLab(gitlabConfig(stub))
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
