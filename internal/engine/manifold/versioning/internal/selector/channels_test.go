package selector

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
)

func TestChannelsOf(t *testing.T) {
	testCases := []struct {
		name string
		snap domain.RefSnapshot
		want []models.ChannelInfo
	}{
		{
			name: "ordered channels, then pointer tags, never the branch once tags exist",
			snap: sharedSnapshot(),
			want: []models.ChannelInfo{
				{Name: "stable", Kind: "ordered", Latest: "stable-26.5.1", Count: 3, Members: []string{"stable-26.5.1", "v1.3.0", "v1.2.0"}},
				{Name: "beta", Kind: "ordered", Latest: "beta-26.5-4", Count: 2, Members: []string{"beta-26.5-4", "beta-26.5-1"}},
				{Name: "latest", Kind: "ordered", Latest: "v1.0-latest", Count: 1, Members: []string{"v1.0-latest"}},
				{Name: "nightly", Kind: "pointer", Latest: "nightly"},
				{Name: "nightly-latest", Kind: "pointer", Latest: "nightly-latest"},
			},
		},
		{
			name: "head branch fallback when there are no tags",
			snap: domain.RefSnapshot{Branches: map[string]string{"develop": "c"}, Head: "develop"},
			want: []models.ChannelInfo{
				{Name: "develop", Kind: "pointer", Latest: "develop", IsDefaultBranchFallback: true},
			},
		},
		{
			name: "no tags and no head",
			snap: domain.RefSnapshot{Branches: map[string]string{"develop": "c"}},
			want: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ChannelsOf(tc.snap))
		})
	}
}

func TestChannelsOf_DeterministicAcrossCalls(t *testing.T) {
	snap := domain.RefSnapshot{Tags: map[string]string{"v1.0": "a", "1.0": "b", "v1.0.0": "c"}}
	first := ChannelsOf(snap)
	for i := 0; i < 20; i++ {
		require.Equal(t, first, ChannelsOf(snap))
	}
}

func TestLatestStable(t *testing.T) {
	latest, ok := LatestStable(sharedSnapshot())
	assert.True(t, ok)
	assert.Equal(t, "stable-26.5.1", latest)

	_, ok = LatestStable(domain.RefSnapshot{Tags: map[string]string{"nightly": "n"}})
	assert.False(t, ok)
}

func TestDefaultBranch(t *testing.T) {
	testCases := []struct {
		name       string
		snap       domain.RefSnapshot
		wantBranch string
		wantCommit string
		wantOK     bool
	}{
		{name: "head branch", snap: sharedSnapshot(), wantBranch: "develop", wantCommit: "c-develop", wantOK: true},
		{name: "no head", snap: domain.RefSnapshot{Branches: map[string]string{"develop": "c"}}},
		{name: "head not listed", snap: domain.RefSnapshot{Head: "main"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			branch, commit, ok := DefaultBranch(tc.snap)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantBranch, branch)
			assert.Equal(t, tc.wantCommit, commit)
		})
	}
}
