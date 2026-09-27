package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold"
	"github.com/rabbytesoftware/quiver.core/internal/engine/provider"
)

type forgeHost struct {
	provider.Provider
	forge provider.Forge
}

func adaptHost(
	p provider.Provider,
) manifold.Host {
	forge, ok := p.(provider.Forge)
	if !ok {
		return p
	}
	return forgeHost{Provider: p, forge: forge}
}

func (h forgeHost) ReleaseAssets(
	ctx context.Context,
	ns domain.Namespace,
	tag string,
) ([]manifold.Asset, error) {
	assets, err := h.forge.ReleaseAssets(ctx, ns, tag)
	if err != nil {
		return nil, translateForgeErr(err)
	}

	out := make([]manifold.Asset, 0, len(assets))
	for _, a := range assets {
		out = append(out, manifold.Asset{
			Name:   a.Name,
			URL:    a.URL,
			Size:   a.Size,
			Digest: a.Digest,
		})
	}
	return out, nil
}

func (h forgeHost) RepoPage(
	ctx context.Context,
	ns domain.Namespace,
) (manifold.RepoPage, error) {
	page, err := h.forge.RepoPage(ctx, ns)
	if err != nil {
		return manifold.RepoPage{}, translateForgeErr(err)
	}

	return manifold.RepoPage{
		Description:       page.Description,
		SocialImage:       page.SocialImage,
		CustomSocialImage: page.CustomSocialImage,
		OwnerIsOrg:        page.OwnerIsOrg,
		OwnerAvatar:       page.OwnerAvatar,
	}, nil
}

func (h forgeHost) RawFile(
	ctx context.Context,
	ns domain.Namespace,
	ref string,
	path string,
) ([]byte, error) {
	body, err := h.forge.RawFile(ctx, ns, ref, path)
	if err != nil {
		return nil, translateForgeErr(err)
	}
	return body, nil
}

func translateForgeErr(
	err error,
) error {
	if errors.Is(err, provider.ErrRawNotFound) {
		return fmt.Errorf("engine container: %w", manifold.ErrRawNotFound)
	}
	if errors.Is(err, provider.ErrUnexpectedPage) {
		return fmt.Errorf("engine container: %w", manifold.ErrUnexpectedPage)
	}
	if errors.Is(err, provider.ErrReleaseNotFound) {
		return fmt.Errorf("engine container: %w", manifold.ErrReleaseNotFound)
	}
	return err
}
