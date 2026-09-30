package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type githubRepoMetadata struct {
	Description string `json:"description"`
	Owner       struct {
		AvatarURL string `json:"avatar_url"`
	} `json:"owner"`
}

// RepoMetadata asks GitHub's repository API once per repository and remembers
// the answer, so no number of tags, platforms or probes spends a second
// request of the unauthenticated quota.
func (p *githubProvider) RepoMetadata(
	ctx context.Context,
	ns domain.Namespace,
) (domain.RepoMetadata, error) {
	if p.repoAPIURL == "" {
		return domain.RepoMetadata{}, fmt.Errorf("provider %s: %w", p.name, ErrNoRepoMetadata)
	}
	rawURL, err := fillRepoTemplate(p.repoAPIURL, ns, "", "")
	if err != nil {
		return domain.RepoMetadata{}, fmt.Errorf("provider %s: repo metadata: %w", p.name, err)
	}

	return p.metadata.get(ctx, string(ns.BareNamespace()), func(
		ctx context.Context,
	) (domain.RepoMetadata, error) {
		return p.fetchRepoMetadata(ctx, rawURL)
	})
}

func (p *githubProvider) fetchRepoMetadata(
	ctx context.Context,
	rawURL string,
) (domain.RepoMetadata, error) {
	resp, err := p.transport.fetch(ctx, rawURL, p.headers())
	if err != nil {
		return domain.RepoMetadata{}, fmt.Errorf("provider %s: repo metadata: %w", p.name, err)
	}
	if err := p.transport.classify(resp); err != nil {
		return domain.RepoMetadata{}, fmt.Errorf("provider %s: repo metadata: %w", p.name, err)
	}

	var decoded githubRepoMetadata
	if err := json.Unmarshal(resp.Body, &decoded); err != nil {
		return domain.RepoMetadata{}, fmt.Errorf("provider %s: decode repo metadata: %w", p.name, err)
	}
	return domain.RepoMetadata{
		Description: strings.TrimSpace(decoded.Description),
		AvatarURL:   strings.TrimSpace(decoded.Owner.AvatarURL),
	}, nil
}
