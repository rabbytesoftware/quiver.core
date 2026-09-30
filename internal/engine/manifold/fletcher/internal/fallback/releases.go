package fallback

import (
	"context"
	"errors"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	manifoldModels "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver/resolvers"
)

func (f *fallback) latestStable(
	ctx context.Context,
	ns domain.Namespace,
) (string, error) {
	tag, err := f.releases.ResolveLatestStable(ctx, ns)
	return tag, lookupFailure(err)
}

func (f *fallback) latestUnstable(
	ctx context.Context,
	ns domain.Namespace,
) (string, error) {
	channels, err := f.releases.ListChannels(ctx, ns)
	if err != nil {
		return "", lookupFailure(err)
	}
	for _, channel := range channels {
		if channel.Name == resolvers.StableChannel || channel.IsDefaultBranchFallback {
			continue
		}
		return channel.Latest, nil
	}
	return "", nil
}

func lookupFailure(
	err error,
) error {
	if errors.Is(err, manifoldModels.ErrNoLatestStable) || errors.Is(err, manifoldModels.ErrNoTagInChannel) {
		return nil
	}
	return err
}
