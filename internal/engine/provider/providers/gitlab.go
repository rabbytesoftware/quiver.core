package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type gitlabProvider struct {
	transport transport
	searchURL string
}

// NewGitLab builds the provider speaking GitLab's search dialect.
func NewGitLab(
	cfg Config,
) Provider {
	return &gitlabProvider{
		transport: newTransport(cfg),
		searchURL: cfg.SearchURL,
	}
}

type gitlabProject struct {
	PathWithNamespace string `json:"path_with_namespace"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	Stars             int    `json:"star_count"`
	DefaultBranch     string `json:"default_branch"`
}

func (p *gitlabProvider) Host() string {
	return p.transport.host
}

func (p *gitlabProvider) Search(
	ctx context.Context,
	req SearchRequest,
) ([]Candidate, error) {
	return searchEachTopic(ctx, req.Topics, req.Limit,
		func(ctx context.Context, topic string) ([]Candidate, error) {
			return p.searchOne(ctx, req.Text, topic, req.Limit)
		})
}

func (p *gitlabProvider) searchOne(
	ctx context.Context,
	text string,
	topic string,
	limit int,
) ([]Candidate, error) {
	rawURL := withLimit(buildSearchURL(p.searchURL, text, topic), limit)

	body, err := p.transport.get(ctx, rawURL, p.headers())
	if err != nil {
		return nil, err
	}

	var projects []gitlabProject
	if err := json.Unmarshal(body, &projects); err != nil {
		return nil, fmt.Errorf("provider %s: decode search response: %w", p.transport.host, err)
	}

	candidates := make([]Candidate, 0, len(projects))
	for _, project := range projects {
		candidate, ok := candidateOf(
			p.transport.host,
			project.PathWithNamespace,
			project.Name,
			project.Description,
			project.Stars,
			project.DefaultBranch,
		)
		if !ok {
			continue
		}
		candidates = append(candidates, candidate)
	}
	return truncate(candidates, limit), nil
}

// headers carries no credentials. Quiver authenticates to no git host: every
// search is anonymous, so there is no token to send and none is asked for.
func (p *gitlabProvider) headers() http.Header {
	headers := http.Header{}
	headers.Set("Accept", "application/json")
	return headers
}
