package manifold

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	resolvers "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver/resolvers"
)

// snapshotCacheEntry is one Snapshot result, timestamped so cachedSnapshot
// can tell a still-fresh hit from one due for a live re-check.
type snapshotCacheEntry struct {
	snap     domain.RefSnapshot
	cachedAt time.Time
}

func (m *manifold) Snapshot(
	ctx context.Context,
	ns domain.Namespace,
) (domain.RefSnapshot, error) {
	bare := ns.BareNamespace()
	if cached, ok := m.cachedSnapshot(bare); ok {
		return cached, nil
	}
	return m.liveSnapshot(ctx, bare)
}

func (m *manifold) FreshSnapshot(
	ctx context.Context,
	ns domain.Namespace,
) (domain.RefSnapshot, error) {
	return m.liveSnapshot(ctx, ns.BareNamespace())
}

func (m *manifold) liveSnapshot(
	ctx context.Context,
	bare domain.Namespace,
) (domain.RefSnapshot, error) {
	snap, err := m.constraint.Refs(ctx, bare)
	if err != nil {
		return domain.RefSnapshot{}, fmt.Errorf("manifold: snapshot %s: %w", bare, err)
	}

	m.snapshotCache.Store(bare, snapshotCacheEntry{snap: snap, cachedAt: m.clock()})
	return snap, nil
}

// cachedSnapshot returns the still-fresh snapshot of bare, if one exists. The
// maps inside it are shared with the cache, so callers treat it as read-only.
func (m *manifold) cachedSnapshot(
	bare domain.Namespace,
) (domain.RefSnapshot, bool) {
	v, ok := m.snapshotCache.Load(bare)
	if !ok {
		return domain.RefSnapshot{}, false
	}
	entry, _ := v.(snapshotCacheEntry)
	if m.clock().Sub(entry.cachedAt) > m.cacheTTL {
		return domain.RefSnapshot{}, false
	}
	return entry.snap, true
}

// ChannelsOf buckets a snapshot's tags into channels: ordered channels by
// classified name, every unclassified tag as its own pointer channel, and the
// HEAD branch as a fallback pointer channel only when there are no tags at all.
func ChannelsOf(
	snap domain.RefSnapshot,
) []ChannelInfo {
	tags := sortedTags(snap)

	var channels []ChannelInfo
	consumed := make(map[string]bool)
	for _, channel := range resolvers.ChannelsPresent(tags) {
		sorted := resolvers.SortInChannel(tags, channel)
		for _, t := range sorted {
			consumed[t] = true
		}
		channels = append(channels, ChannelInfo{
			Name:    channel,
			Kind:    "ordered",
			Latest:  sorted[0],
			Count:   len(sorted),
			Members: sorted,
		})
	}

	for _, tag := range tags {
		if !consumed[tag] {
			channels = append(channels, ChannelInfo{Name: tag, Kind: "pointer", Latest: tag})
		}
	}

	if len(tags) == 0 && snap.Head != "" {
		channels = append(channels, ChannelInfo{
			Name: snap.Head, Kind: "pointer", Latest: snap.Head,
			IsDefaultBranchFallback: true,
		})
	}

	sortChannels(channels)
	return channels
}

// sortedTags lists a snapshot's tag names in a fixed order, so ranking ties
// inside a channel never depend on map iteration.
func sortedTags(
	snap domain.RefSnapshot,
) []string {
	tags := make([]string, 0, len(snap.Tags))
	for tag := range snap.Tags {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}
