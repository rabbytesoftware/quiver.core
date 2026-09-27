package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// githubReleaseMarker precedes the ref in the redirect GitHub's latest-release
// permalink answers with.
const githubReleaseMarker = "/releases/tag/"

type githubProvider struct {
	host
	searchURL         string
	expandedAssetsURL string
	repoPageURL       string
	orgURL            string
	avatarURL         string
}

// NewGitHub builds the provider answering for a GitHub host.
func NewGitHub(
	cfg Config,
) Provider {
	return &githubProvider{
		host:              newHost(cfg, githubReleaseMarker),
		searchURL:         cfg.SearchURL,
		expandedAssetsURL: cfg.ExpandedAssetsURL,
		repoPageURL:       cfg.RepoPageURL,
		orgURL:            cfg.OrgURL,
		avatarURL:         cfg.AvatarURL,
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
		return p.searchOne(ctx, unmarkedGithubQuery(req.Text, req.MinStars), "", req.Limit)
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

func unmarkedGithubQuery(
	text string,
	minStars int,
) string {
	parts := make([]string, 0, 4)
	if text != "" {
		parts = append(parts, text)
	}
	parts = append(parts, "fork:false", "archived:false", fmt.Sprintf("stars:>=%d", minStars))
	return strings.Join(parts, " ")
}

func (p *githubProvider) RawFile(
	ctx context.Context,
	ns domain.Namespace,
	ref string,
	filePath string,
) ([]byte, error) {
	rawURL, err := p.RawFileURL(ns, ref, filePath)
	if err != nil {
		return nil, fmt.Errorf("provider %s: raw file: %w", p.name, err)
	}

	resp, err := p.transport.fetch(ctx, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("provider %s: raw file: %w", p.name, err)
	}
	if resp.Status == http.StatusNotFound {
		return nil, fmt.Errorf("provider %s: raw file: %w", p.name, ErrRawNotFound)
	}
	if resp.Status < http.StatusOK || resp.Status >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("provider %s: raw file: http %d", p.name, resp.Status)
	}
	return resp.Body, nil
}

func (p *githubProvider) ReleaseAssets(
	ctx context.Context,
	ns domain.Namespace,
	tag string,
) ([]Asset, error) {
	rawURL, err := p.expandedAssetsURLFor(ns, tag)
	if err != nil {
		return nil, err
	}

	resp, err := p.transport.fetch(ctx, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("provider %s: release assets: %w", p.name, err)
	}
	if resp.Status == http.StatusNotFound {
		return nil, fmt.Errorf("provider %s: release assets: %w", p.name, ErrReleaseNotFound)
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

func (p *githubProvider) expandedAssetsURLFor(
	ns domain.Namespace,
	tag string,
) (string, error) {
	if p.expandedAssetsURL == "" {
		return "", fmt.Errorf("provider %s: release assets: %w", p.name, ErrNoRawURL)
	}

	user, repo, err := repositoryOf(ns)
	if err != nil {
		return "", fmt.Errorf("provider %s: release assets: %w", p.name, err)
	}

	return strings.NewReplacer(
		"{user}", url.PathEscape(user),
		"{repo}", url.PathEscape(repo),
		"{tag}", url.PathEscape(tag),
	).Replace(p.expandedAssetsURL), nil
}

func (p *githubProvider) RepoPage(
	ctx context.Context,
	ns domain.Namespace,
) (RepoPage, error) {
	user, repo, err := repositoryOf(ns)
	if err != nil {
		return RepoPage{}, fmt.Errorf("provider %s: repo page: %w", p.name, err)
	}
	if p.repoPageURL == "" {
		return RepoPage{}, fmt.Errorf("provider %s: repo page: %w", p.name, ErrNoRawURL)
	}

	pageURL := strings.NewReplacer(
		"{user}", url.PathEscape(user),
		"{repo}", url.PathEscape(repo),
	).Replace(p.repoPageURL)
	resp, err := p.transport.fetch(ctx, pageURL, nil)
	if err != nil {
		return RepoPage{}, fmt.Errorf("provider %s: repo page: %w", p.name, err)
	}
	if resp.Status < http.StatusOK || resp.Status >= http.StatusMultipleChoices {
		return RepoPage{}, fmt.Errorf("provider %s: repo page: http %d", p.name, resp.Status)
	}

	page, err := parseRepoPage(resp.Body, user+"/"+repo)
	if err != nil {
		return RepoPage{}, fmt.Errorf("provider %s: repo page: %w", p.name, err)
	}

	page.OwnerIsOrg, err = p.ownerIsOrg(ctx, user)
	if err != nil {
		return RepoPage{}, fmt.Errorf("provider %s: repo page: %w", p.name, err)
	}
	page.OwnerAvatar = p.ownerAvatar(ctx, user)

	return page, nil
}

func (p *githubProvider) ownerIsOrg(
	ctx context.Context,
	user string,
) (bool, error) {
	if p.orgURL == "" {
		return false, nil
	}
	orgURL := strings.ReplaceAll(p.orgURL, "{user}", url.PathEscape(user))

	resp, err := p.transport.redirect(ctx, orgURL)
	if err != nil {
		return false, fmt.Errorf("owner kind: %w", err)
	}
	if resp.Status == http.StatusNotFound {
		return false, nil
	}
	if resp.Status >= http.StatusMultipleChoices && resp.Status < http.StatusBadRequest {
		return true, nil
	}
	if resp.Status == http.StatusOK {
		return true, nil
	}
	return false, fmt.Errorf("owner kind: %s answered http %d", orgURL, resp.Status)
}

func (p *githubProvider) ownerAvatar(
	ctx context.Context,
	user string,
) string {
	if p.avatarURL == "" {
		return ""
	}
	avatarURL := strings.ReplaceAll(p.avatarURL, "{user}", url.PathEscape(user))

	resp, err := p.transport.redirect(ctx, avatarURL)
	if err != nil {
		return avatarURL
	}
	if location := resp.Headers.Get("Location"); location != "" {
		return location
	}
	return avatarURL
}
