package selector

import (
	"sort"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
	resolvers "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver/resolvers"
)

// ChannelsOf buckets a snapshot's tags into channels: ordered channels by
// classified name, every unclassified tag as its own pointer channel, and the
// HEAD branch as a fallback pointer channel only when there are no tags at all.
func ChannelsOf(
	snap domain.RefSnapshot,
) []models.ChannelInfo {
	tags := SortedTags(snap)

	var channels []models.ChannelInfo
	consumed := make(map[string]bool)
	for _, channel := range resolvers.GroupChannels(tags) {
		for _, t := range channel.Members {
			consumed[t] = true
		}
		channels = append(channels, models.ChannelInfo{
			Name:    channel.Name,
			Kind:    "ordered",
			Latest:  channel.Members[0],
			Count:   len(channel.Members),
			Members: channel.Members,
		})
	}

	for _, tag := range tags {
		if !consumed[tag] {
			channels = append(channels, models.ChannelInfo{Name: tag, Kind: "pointer", Latest: tag})
		}
	}

	if len(tags) == 0 && snap.Head != "" {
		channels = append(channels, models.ChannelInfo{
			Name: snap.Head, Kind: "pointer", Latest: snap.Head,
			IsDefaultBranchFallback: true,
		})
	}

	sortChannels(channels)
	return channels
}

// SortedTags lists a snapshot's tag names in a fixed order, so ranking ties
// inside a channel never depend on map iteration.
func SortedTags(
	snap domain.RefSnapshot,
) []string {
	tags := make([]string, 0, len(snap.Tags))
	for tag := range snap.Tags {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

// sortChannels orders a ListChannels result deterministically: stable first
// (if present), then ordered channels alphabetically by name, then pointer
// channels alphabetically by name. Map iteration order (over ListChannels's
// internal "ordered" bucket) is otherwise randomized by Go on every call.
func sortChannels(
	channels []models.ChannelInfo,
) {
	sort.Slice(channels, func(i, j int) bool {
		a, b := channels[i], channels[j]
		aStable := a.Name == resolvers.StableChannel
		bStable := b.Name == resolvers.StableChannel
		if aStable != bStable {
			return aStable
		}
		if aStable {
			return false
		}
		if a.Kind != b.Kind {
			return a.Kind == "ordered"
		}
		return a.Name < b.Name
	})
}

// LatestStable names the latest tag of snap's stable channel.
func LatestStable(
	snap domain.RefSnapshot,
) (string, bool) {
	channel, ok := FindChannel(resolvers.StableChannel, snap)
	return channel.Latest, ok
}

// DefaultBranch names snap's HEAD branch and the commit it is at.
func DefaultBranch(
	snap domain.RefSnapshot,
) (branch, commit string, ok bool) {
	commit, ok = snap.Branches[snap.Head]
	if snap.Head == "" || !ok {
		return "", "", false
	}
	return snap.Head, commit, true
}
