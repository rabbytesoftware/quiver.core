package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// githubReleaseMarker precedes the ref in the redirect GitHub's latest-release
// permalink answers with.
const githubReleaseMarker = "/releases/tag/"

type githubProvider struct {
	host
	searchURL string
}

// NewGitHub builds the provider answering for a GitHub host.
func NewGitHub(
	cfg Config,
) Provider {
	return &githubProvider{
		host:      newHost(cfg, githubReleaseMarker),
		searchURL: cfg.SearchURL,
	}
}

func (p *githubProvider) CanSearch() bool {
	return p.searchURL != ""
}

type githubRepo struct {
	FullName      string `json:"full_name"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Stars         int    `json:"stargazers_count"`
	DefaultBranch string `json:"default_branch"`
}

type githubSearchResponse struct {
	Items []githubRepo `json:"items"`
}

func (p *githubProvider) Search(
	ctx context.Context,
	req SearchRequest,
) ([]Candidate, error) {
	if !p.CanSearch() {
		return p.host.Search(ctx, req)
	}

	return searchEachTopic(ctx, req.Topics, req.Limit,
		func(ctx context.Context, topic string) ([]Candidate, error) {
			return p.searchOne(ctx, req.Text, topic, req.Limit)
		})
}

func (p *githubProvider) searchOne(
	ctx context.Context,
	text string,
	topic string,
	limit int,
) ([]Candidate, error) {
	rawURL := withLimit(buildSearchURL(p.searchURL, githubQuery(text, topic), ""), limit)

	body, err := p.transport.get(ctx, rawURL, p.headers())
	if err != nil {
		return nil, err
	}

	var decoded githubSearchResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("provider %s: decode search response: %w", p.name, err)
	}

	candidates := make([]Candidate, 0, len(decoded.Items))
	for _, repo := range decoded.Items {
		candidate, ok := candidateOf(
			p.name,
			repo.FullName,
			repo.Name,
			repo.Description,
			repo.Stars,
			repo.DefaultBranch,
		)
		if !ok {
			continue
		}
		candidates = append(candidates, candidate)
	}
	return truncate(candidates, limit), nil
}

func (p *githubProvider) headers() http.Header {
	headers := http.Header{}
	headers.Set("Accept", "application/vnd.github+json")
	return headers
}

// githubQuery folds a single marker into q as a topic qualifier, which GitHub
// intersects: every extra topic in one query narrows the result set. One marker
// per request: the union across markers is assembled by searchEachTopic.
func githubQuery(
	text string,
	topic string,
) string {
	parts := make([]string, 0, 2)
	if text != "" {
		parts = append(parts, text)
	}
	if topic != "" {
		parts = append(parts, "topic:"+topic)
	}
	return strings.Join(parts, " ")
}
