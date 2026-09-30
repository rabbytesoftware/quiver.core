package manifold

import (
	"context"
	"fmt"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
)

// snapshotReleases answers Fletcher's release questions from the manifold's
// own ref snapshot, so drafting a manifest reads the same refs a selector
// resolves against.
type snapshotReleases struct {
	m *manifold
}

func (r snapshotReleases) ResolveLatestStable(
	ctx context.Context,
	ns domain.Namespace,
) (string, error) {
	snap, err := r.m.Snapshot(ctx, ns)
	if err != nil {
		return "", fmt.Errorf("manifold: latest stable: %w", err)
	}
	channel, ok := findChannel(StableChannel, snap)
	if !ok {
		return "", fmt.Errorf("manifold: latest stable %s: %w", ns, models.ErrNoLatestStable)
	}
	return channel.Latest, nil
}

func (r snapshotReleases) ListChannels(
	ctx context.Context,
	ns domain.Namespace,
) ([]ChannelInfo, error) {
	return r.m.ListChannels(ctx, ns)
}

func (r snapshotReleases) ResolveDefaultBranch(
	ctx context.Context,
	ns domain.Namespace,
) (branch, hash string, err error) {
	snap, err := r.m.Snapshot(ctx, ns)
	if err != nil {
		return "", "", fmt.Errorf("manifold: default branch: %w", err)
	}
	commit, ok := snap.Branches[snap.Head]
	if snap.Head == "" || !ok {
		return "", "", fmt.Errorf("manifold: default branch %s: %w", ns, ErrUnknownSelector)
	}
	return snap.Head, commit, nil
}
