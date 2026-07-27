package usecases

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestNormalise_EmptySet(t *testing.T) {
	assert.Empty(t, normalise(nil))
	assert.Empty(t, normalise([]float64{}))
}

func TestNormalise_SingleElement(t *testing.T) {
	got := normalise(positionScores(1))

	require.Len(t, got, 1)
	assert.False(t, math.IsNaN(got[0]), "a single-element set must not divide by a zero range")
	assert.InDelta(t, 1.0, got[0], 1e-9)
}

func TestNormalise_AllEqualMapToOne(t *testing.T) {
	got := normalise([]float64{7, 7, 7})

	require.Len(t, got, 3)
	for _, v := range got {
		assert.InDelta(t, 1.0, v, 1e-9)
	}
}

func TestNormalise_DescendsLinearly(t *testing.T) {
	got := normalise(positionScores(5))

	require.Len(t, got, 5)
	for i, want := range []float64{1.0, 0.75, 0.5, 0.25, 0.0} {
		assert.InDelta(t, want, got[i], 1e-9, "index %d", i)
	}
}

func TestNormalise_ArbitraryScoresMapToUnitRange(t *testing.T) {
	testCases := []struct {
		name string
		in   []float64
		want []float64
	}{
		{name: "already ordered", in: []float64{4, 2, 0}, want: []float64{1, 0.5, 0}},
		{name: "negative bm25 values", in: []float64{-1, -3, -5}, want: []float64{1, 0.5, 0}},
		{name: "unordered input", in: []float64{0, 10, 5}, want: []float64{0, 1, 0.5}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalise(tc.in)
			require.Len(t, got, len(tc.want))
			for i, want := range tc.want {
				assert.InDelta(t, want, got[i], 1e-9, "index %d", i)
			}
		})
	}
}

func TestPositionScores_FirstRowRanksHighest(t *testing.T) {
	assert.Empty(t, positionScores(0))
	assert.Equal(t, []float64{2, 1, 0}, positionScores(3))
}

func TestApplyBoosts_ExactNameBeatsHighStarFuzzyMatch(t *testing.T) {
	exact := models.SearchResult{Name: "kafka", Stars: 0}
	popular := models.SearchResult{Name: "kafka-ui", Stars: 400000}

	// Equal textual relevance, and equal curation: only the name and the stars
	// separate them.
	exactScore := applyBoosts(0.5, exact, "kafka", false)
	popularScore := applyBoosts(0.5, popular, "kafka", false)

	assert.Greater(t, exactScore, popularScore)
	// No star count can ever make up an exact name match.
	assert.Less(t, starBoostMax, exactNameBoost)
}

// TestApplyBoosts_ExactNameSurvivesTheWorstPosition is the worst case the
// comment on exactNameBoost promises to survive, and the one the test above
// cannot see: it holds base equal for both rows, so it compares boosts in
// isolation and never crosses the normalised span. Here relevance and
// popularity both favour the stranger — it ranks first while the exactly named
// arrow ranks last — so the exact-name boost has to beat the full span plus the
// star boost, not merely the star boost.
func TestApplyBoosts_ExactNameSurvivesTheWorstPosition(t *testing.T) {
	bases := normalise(positionScores(2))

	stranger := applyBoosts(
		bases[0],
		models.SearchResult{Name: "widget-ui", Stars: 400000},
		"widget",
		false,
	)
	exact := applyBoosts(
		bases[1],
		models.SearchResult{Name: "widget", Stars: 0},
		"widget",
		false,
	)

	assert.Greater(t, exact, stranger,
		"a 400k-star stranger must not bury the arrow the query named")
	assert.Greater(t, exactNameBoost, 1.0+starBoostMax,
		"exactNameBoost must exceed the whole normalised span plus every rival boost")
}

func TestApplyBoosts_ExactNameIsCaseInsensitive(t *testing.T) {
	r := models.SearchResult{Name: "Kafka"}

	// Asserted against the constant, not its value: this test is about casing
	// and trimming, and should not fail when the boost is retuned.
	assert.InDelta(t, exactNameBoost, applyBoosts(0.0, r, "  kAfKa  ", false), 1e-9)
	assert.InDelta(t, 0.0, applyBoosts(0.0, r, "kafk", false), 1e-9)
}

func TestApplyBoosts_FollowedCollectionBeatsEqualStranger(t *testing.T) {
	member := models.SearchResult{Name: "one", Namespace: domain.Namespace("github.com/a/one")}
	stranger := models.SearchResult{Name: "two", Namespace: domain.Namespace("github.com/b/two")}

	followedScore := applyBoosts(0.5, member, "query", true)
	strangerScore := applyBoosts(0.5, stranger, "query", false)

	assert.Greater(t, followedScore, strangerScore)
	assert.InDelta(t, followedBoost, followedScore-strangerScore, 1e-9)
}

func TestApplyBoosts_StarBoostSaturates(t *testing.T) {
	base := models.SearchResult{Name: "thing"}

	starsOf := func(n int) float64 {
		r := base
		r.Stars = n
		return applyBoosts(0.0, r, "query", false)
	}

	// Capped: no star count can add more than starBoostMax.
	assert.InDelta(t, starBoostMax, starsOf(400000), 1e-9)
	assert.InDelta(t, starBoostMax, starsOf(4000000), 1e-9)

	// Saturating: a tenfold jump at the top of the range moves the score far
	// less than the same jump near the bottom, and never enough to overturn an
	// exact name match.
	topDecade := starsOf(400000) - starsOf(40000)
	bottomDecade := starsOf(40000) - starsOf(4000)
	assert.Less(t, topDecade, bottomDecade)
	assert.Less(t, topDecade, exactNameBoost)
	assert.Less(t, starBoostMax, exactNameBoost)
}

func TestApplyBoosts_ZeroStarsAddsNothing(t *testing.T) {
	testCases := []struct {
		name  string
		stars int
	}{
		{name: "zero", stars: 0},
		{name: "negative", stars: -5},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := models.SearchResult{Name: "thing", Stars: tc.stars}
			assert.InDelta(t, 0.42, applyBoosts(0.42, r, "query", false), 1e-9)
		})
	}
}

func TestSortScored_DescendingScoreThenNamespace(t *testing.T) {
	items := []scored{
		{result: models.SearchResult{Namespace: domain.Namespace("github.com/b/two")}, score: 0.5},
		{result: models.SearchResult{Namespace: domain.Namespace("github.com/a/one")}, score: 0.5},
		{result: models.SearchResult{Namespace: domain.Namespace("github.com/c/three")}, score: 0.9},
	}

	sortScored(items)

	assert.Equal(t, domain.Namespace("github.com/c/three"), items[0].result.Namespace)
	assert.Equal(t, domain.Namespace("github.com/a/one"), items[1].result.Namespace)
	assert.Equal(t, domain.Namespace("github.com/b/two"), items[2].result.Namespace)
}
