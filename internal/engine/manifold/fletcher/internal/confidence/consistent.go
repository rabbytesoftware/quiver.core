package confidence

import (
	"maps"
	"slices"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher/internal/picker"
)

func consistent(
	picks map[domain.OS]picker.Pick,
) (map[domain.OS]picker.Pick, bool) {
	product := dominantProduct(picks)
	kept := make(map[domain.OS]picker.Pick, len(picks))
	for platform, pick := range picks {
		if pick.NameMatch || (pick.Accepted && pick.Product != "" && pick.Product == product) {
			kept[platform] = pick
		}
	}
	return kept, len(kept) < len(picks)
}

func dominantProduct(
	picks map[domain.OS]picker.Pick,
) string {
	named := map[string]int{}
	all := map[string]int{}
	for _, pick := range picks {
		all[pick.Product]++
		if pick.NameMatch {
			named[pick.Product]++
		}
	}
	if len(named) > 0 {
		return mostCommon(named)
	}
	return mostCommon(all)
}

func mostCommon(
	counts map[string]int,
) string {
	products := slices.Sorted(maps.Keys(counts))
	best := ""
	top := 0
	tied := false
	for _, product := range products {
		if counts[product] > top {
			best, top, tied = product, counts[product], false
			continue
		}
		if counts[product] == top {
			tied = true
		}
	}
	if tied {
		return ""
	}
	return best
}
