package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type githubProvider struct {
	transport transport
	searchURL string
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

func (p *githubProvider) Host() string {
	return p.transport.host
}

func (p *githubProvider) Search(
	ctx context.Context,
	req SearchRequest,
) ([]Candidate, error) {
	rawURL := withLimit(buildSearchURL(p.searchURL, githubQuery(req), ""), req.Limit)

	body, err := p.transport.get(ctx, rawURL, p.headers())
	if err != nil {
		return nil, err
	}

	var decoded githubSearchResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("provider %s: decode search response: %w", p.transport.host, err)
	}

	candidates := make([]Candidate, 0, len(decoded.Items))
	for _, repo := range decoded.Items {
		candidate, ok := candidateOf(
			p.transport.host,
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
	return truncate(candidates, req.Limit), nil
}

func (p *githubProvider) headers() http.Header {
	headers := http.Header{}
	headers.Set("Accept", "application/vnd.github+json")
	if p.transport.token != "" {
		headers.Set("Authorization", "Bearer "+p.transport.token)
	}
	return headers
}

// githubQuery folds the markers into q as topic qualifiers, which GitHub
// intersects: every extra topic narrows the result set.
func githubQuery(
	req SearchRequest,
) string {
	parts := make([]string, 0, len(req.Topics)+1)
	if req.Text != "" {
		parts = append(parts, req.Text)
	}
	for _, topic := range req.Topics {
		parts = append(parts, "topic:"+topic)
	}
	return strings.Join(parts, " ")
}
