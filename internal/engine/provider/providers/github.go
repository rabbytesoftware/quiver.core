package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// githubReleaseMarker precedes the ref in the redirect GitHub's latest-release
// permalink answers with.
const githubReleaseMarker = "/releases/tag/"

type githubProvider struct {
	host
	searchURL         string
	expandedAssetsURL string
	repoAPIURL        string
	metadata          *repoMetadataCache
}

// NewGitHub builds the provider answering for a GitHub host.
func NewGitHub(
	cfg Config,
) Provider {
	return &githubProvider{
		host:              newHost(cfg, githubReleaseMarker),
		searchURL:         cfg.SearchURL,
		expandedAssetsURL: cfg.ExpandedAssetsURL,
		repoAPIURL:        cfg.RepoAPIURL,
		metadata:          newRepoMetadataCache(newTransport(cfg).now),
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
	if req.Unmarked {
		query := unmarkedGithubQuery(req.Text, req.MinStars, req.MaxStars, pushedSince(p.transport.now(), req.PushedWithin))
		return p.searchOne(ctx, query, req.Sort, req.Limit)
	}

	return searchEachTopic(ctx, req.Topics, req.Limit,
		func(ctx context.Context, topic string) ([]Candidate, error) {
			return p.searchOne(ctx, githubQuery(req.Text, topic), "", req.Limit)
		})
}

func (p *githubProvider) searchOne(
	ctx context.Context,
	query string,
	sort string,
	limit int,
) ([]Candidate, error) {
	rawURL := withLimit(withSort(buildSearchURL(p.searchURL, query, ""), sort), limit)

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

// unmarkedGithubQuery builds the query for repositories that carry no discovery
// topic. Text may be empty, in which case the qualifiers alone select the set;
// pushedSince is the zero time when no recency window was asked for.
func unmarkedGithubQuery(
	text string,
	minStars int,
	maxStars int,
	pushedSince time.Time,
) string {
	parts := make([]string, 0, 5)
	if text != "" {
		parts = append(parts, text)
	}
	parts = append(parts, "fork:false", "archived:false", starsQualifier(minStars, maxStars))
	if !pushedSince.IsZero() {
		parts = append(parts, "pushed:>="+pushedSince.UTC().Format(time.DateOnly))
	}
	return strings.Join(parts, " ")
}

func starsQualifier(
	minStars int,
	maxStars int,
) string {
	if maxStars <= 0 {
		return fmt.Sprintf("stars:>=%d", minStars)
	}
	return fmt.Sprintf("stars:%d..%d", minStars, maxStars)
}

func pushedSince(
	now time.Time,
	within time.Duration,
) time.Time {
	if within <= 0 {
		return time.Time{}
	}
	return now.Add(-within)
}

func (p *githubProvider) ReleaseAssets(
	ctx context.Context,
	ns domain.Namespace,
	tag string,
) ([]domain.ReleaseAsset, error) {
	rawURL, err := tagURL(p.expandedAssetsURL, ns, tag)
	if err != nil {
		return nil, fmt.Errorf("provider %s: release assets: %w", p.name, err)
	}

	resp, err := p.transport.fetch(ctx, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("provider %s: release assets: %w", p.name, err)
	}
	if resp.Status == http.StatusNotFound {
		return []domain.ReleaseAsset{}, nil
	}
	if resp.Status < http.StatusOK || resp.Status >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("provider %s: release assets: http %d", p.name, resp.Status)
	}

	assets, err := parseExpandedAssets(resp.Body, rawURL)
	if err != nil {
		return nil, fmt.Errorf("provider %s: release assets: %w", p.name, err)
	}
	return assets, nil
}
