package gather

import (
	"context"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/hosts"
)

func repoMetadataOf(
	ctx context.Context,
	host hosts.Host,
	ns domain.Namespace,
) domain.RepoMetadata {
	meta, err := host.RepoMetadata(ctx, ns)
	if err != nil {
		return domain.RepoMetadata{}
	}
	return meta
}

func (s sources) description() string {
	if s.meta.Description != "" {
		return s.meta.Description
	}
	return s.page.description
}

func (s sources) ownerAvatar() string {
	if s.avatar != "" {
		return s.avatar
	}
	return s.meta.AvatarURL
}
