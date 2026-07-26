package usecases

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
)

// Boost weights, kept together because their ordering is the ranking policy:
// an exact name match states intent outright and outranks everything, curation
// through a followed collection outranks popularity, and popularity is capped
// low enough that a 400k-star repository cannot bury an exactly named arrow.
const (
	exactNameBoost = 1.0
	followedBoost  = 0.5
	starBoostMax   = 0.3
	// starBoostDecades saturates the star boost at 10^5 stars.
	starBoostDecades = 5.0
)

type scored struct {
	result models.SearchResult
	score  float64
}

// positionScores turns a result set's ordering into raw scores, the first row
// highest and the last lowest.
func positionScores(
	n int,
) []float64 {
	scores := make([]float64, n)
	for i := range scores {
		scores[i] = float64(n - 1 - i)
	}
	return scores
}

// normalise min-max maps one result set's raw scores into [0,1].
//
// bm25 is corpus-relative: the catalog index and the vault index hold
// different corpora with different term statistics, so their raw scores are
// not comparable and merging them numerically would be unprincipled.
// Normalising each set against itself is what makes the merge defensible.
//
// Neither store returns the bm25 value it ranked by — both order by it and
// discard it — so callers derive raw scores from result position rather than
// widening two storage interfaces to expose a number that would not be
// comparable across corpora anyway. Position preserves each store's own
// ranking and is already monotonic, which is all the merge needs.
//
// A set with no spread, including a single-element set, maps to 1.0 instead of
// dividing by a zero range.
func normalise(
	scores []float64,
) []float64 {
	out := make([]float64, len(scores))
	if len(scores) == 0 {
		return out
	}

	lowest, highest := scores[0], scores[0]
	for _, s := range scores[1:] {
		lowest = math.Min(lowest, s)
		highest = math.Max(highest, s)
	}

	span := highest - lowest
	for i, s := range scores {
		if span == 0 {
			out[i] = 1
			continue
		}
		out[i] = (s - lowest) / span
	}
	return out
}

// applyBoosts adds the non-textual signals to a normalised relevance score.
// Installed state, provenance and known versions are deliberately absent: they
// describe what can be done with an arrow, not whether it is what was meant.
func applyBoosts(
	base float64,
	r models.SearchResult,
	query string,
	followed bool,
) float64 {
	score := base
	if strings.EqualFold(strings.TrimSpace(query), r.Name) {
		score += exactNameBoost
	}
	if followed {
		score += followedBoost
	}
	return score + starBoost(r.Stars)
}

func starBoost(
	stars int,
) float64 {
	if stars <= 0 {
		return 0
	}
	return starBoostMax * math.Min(1, math.Log10(1+float64(stars))/starBoostDecades)
}

// sortScored orders by descending score, breaking ties on namespace so the
// output does not depend on map iteration or on which store answered first.
func sortScored(
	items []scored,
) {
	slices.SortStableFunc(items, func(a, b scored) int {
		if c := cmp.Compare(b.score, a.score); c != 0 {
			return c
		}
		return cmp.Compare(a.result.Namespace, b.result.Namespace)
	})
}
