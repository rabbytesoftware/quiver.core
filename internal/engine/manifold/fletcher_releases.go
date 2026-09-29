package manifold

import (
	"context"
	"errors"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

type fletcherReleases struct {
	m *manifold
}

func (r fletcherReleases) LatestStable(
	ctx context.Context,
	ns domain.Namespace,
) (string, error) {
	tag, err := r.m.ResolveLatestStable(ctx, ns)
	return tag, lookupFailure(err)
}

func (r fletcherReleases) LatestUnstable(
	ctx context.Context,
	ns domain.Namespace,
) (string, error) {
	channels, err := r.m.ListChannels(ctx, ns)
	if err != nil {
		return "", lookupFailure(err)
	}
	for _, channel := range channels {
		if channel.Name == StableChannel || channel.IsDefaultBranchFallback {
			continue
		}
		return channel.Latest, nil
	}
	return "", nil
}

func (r fletcherReleases) ResolveDefaultBranch(
	ctx context.Context,
	ns domain.Namespace,
) (string, string, error) {
	return r.m.ResolveDefaultBranch(ctx, ns)
}

func lookupFailure(
	err error,
) error {
	if errors.Is(err, ErrNoLatestStable) || errors.Is(err, ErrNoTagInChannel) {
		return nil
	}
	return err
}
