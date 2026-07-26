package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type gitlabProvider struct {
	transport transport
	searchURL string
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
	// GitLab intersects a comma separated topic list, matching GitHub's
	// treatment of repeated topic qualifiers.
	rawURL := withLimit(
		buildSearchURL(p.searchURL, req.Text, strings.Join(req.Topics, ",")),
		req.Limit,
	)

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
	return truncate(candidates, req.Limit), nil
}

// headers carries no credentials: search.token is a GitHub token, and offering
// it to GitLab would turn an anonymous search that works into a 401.
func (p *gitlabProvider) headers() http.Header {
	headers := http.Header{}
	headers.Set("Accept", "application/json")
	return headers
}
