//go:build integration

package fletcher_test

import (
	"context"
	"errors"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
)

const (
	discoveryHost   = "quiver.test"
	fixtureBranch   = "master"
	discoveryStars  = 21
	errNotAskedText = "discovered provider: discovery never asks this"
)

var errNotAsked = errors.New(errNotAskedText)

type discoveredProvider struct {
	fixtures []string
}

func (p *discoveredProvider) Host() string { return discoveryHost }

func (p *discoveredProvider) CanSearch() bool { return true }

func (p *discoveredProvider) Search(
	_ context.Context,
	_ provider.SearchRequest,
) ([]provider.Candidate, error) {
	candidates := make([]provider.Candidate, 0, len(p.fixtures))
	for _, fixture := range p.fixtures {
		candidates = append(candidates, provider.Candidate{
			Namespace:     domain.Namespace(discoveryHost + "/" + fixture),
			Name:          fixture,
			Stars:         discoveryStars,
			Source:        discoveryHost,
			DefaultBranch: fixtureBranch,
		})
	}
	return candidates, nil
}

func (p *discoveredProvider) LatestRelease(
	_ context.Context,
	_ domain.Namespace,
) (string, error) {
	return "", errNotAsked
}

func (p *discoveredProvider) RawFileURL(
	_ domain.Namespace,
	_ string,
	_ string,
) (string, error) {
	return "", errNotAsked
}

func (p *discoveredProvider) BlobFileURL(
	_ domain.Namespace,
	_ string,
	_ string,
) (string, error) {
	return "", nil
}

func (p *discoveredProvider) OwnerAvatarURL(
	_ domain.Namespace,
) string {
	return ""
}

func (p *discoveredProvider) RepoPageURL(
	_ domain.Namespace,
) string {
	return ""
}

func (p *discoveredProvider) RepoMetadata(
	_ context.Context,
	_ domain.Namespace,
) (domain.RepoMetadata, error) {
	return domain.RepoMetadata{}, errNotAsked
}

func (p *discoveredProvider) ReleaseAssets(
	_ context.Context,
	_ domain.Namespace,
	_ string,
) ([]domain.ReleaseAsset, error) {
	return nil, nil
}

func (p *discoveredProvider) DefaultBranches() []string { return nil }
