package resolvers

import (
	"slices"

	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
)

// NewestFirst orders channels so the one whose latest tag carries the highest
// version core comes first; pointer channels follow, in their given order.
func NewestFirst(
	channels []models.ChannelInfo,
) []models.ChannelInfo {
	return slices.SortedStableFunc(slices.Values(channels), newerThan)
}

func newerThan(
	a models.ChannelInfo,
	b models.ChannelInfo,
) int {
	aOrdered := a.Kind == channelKindOrdered
	bOrdered := b.Kind == channelKindOrdered
	if aOrdered != bOrdered {
		return orderedFirst(aOrdered)
	}
	if !aOrdered {
		return 0
	}
	return compareCores(coreOf(b.Latest), coreOf(a.Latest))
}

func orderedFirst(
	aOrdered bool,
) int {
	if aOrdered {
		return -1
	}
	return 1
}

func coreOf(
	tag string,
) [4]int {
	_, core, _, _ := parseTagFull(tag)
	return semverParts(core)
}

const channelKindOrdered = "ordered"
