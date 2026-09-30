package manifold

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// coreReleaseTags is quiver.core's own tag list on 2026-09-30, before and
// after the beta/2026-09-27 branch is merged to master.
func coreReleaseTags(extra ...string) domain.RefSnapshot {
	tags := map[string]string{
		"stable-26.5": "s265", "stable-26.5.1": "s2651",
		"beta-26.5": "b0", "beta-26.5-1": "b1", "beta-26.5-2": "b2", "beta-26.5-3": "b3", "beta-26.5-4": "b4",
		"hotfix-26.5.2": "h2652", "nightly-latest": "n", "beta-2026-09-27": "bdate",
	}
	for _, tag := range extra {
		tags[tag] = "c-" + tag
	}
	return domain.RefSnapshot{Tags: tags, Branches: map[string]string{"develop": "d", "master": "m"}, Head: "develop"}
}

func TestChannelsOf_DatedTagsGroupUnderTheirChannel(t *testing.T) {
	channels := ChannelsOf(coreReleaseTags("stable-2026-09-27"))

	byName := map[string]ChannelInfo{}
	for _, c := range channels {
		byName[c.Name] = c
	}
	require.Contains(t, byName, "beta")
	require.Contains(t, byName, "stable")
	assert.NotContains(t, byName, "beta-2026-09-27", "a dated beta is a beta, not a channel of its own")
	assert.NotContains(t, byName, "stable-2026-09-27")
	assert.Equal(t, "beta-2026-09-27", byName["beta"].Latest)
	assert.Equal(t, "stable-2026-09-27", byName["stable"].Latest)
	assert.Equal(t, []string{"stable-2026-09-27", "stable-26.5.1", "stable-26.5"}, byName["stable"].Members)
	assert.Equal(t, "hotfix-26.5.2", byName["hotfix"].Latest)
	assert.Equal(t, "pointer", byName["nightly-latest"].Kind)
}

func TestDrift_ChannelNeverOffersADowngrade(t *testing.T) {
	testCases := []struct {
		name         string
		kind         domain.SelectorKind
		selector     string
		resolved     domain.Resolved
		snap         domain.RefSnapshot
		wantTarget   domain.Available
		wantOutdated bool
	}{
		{
			name: "a dated stable build is current on the stable channel", kind: domain.SelectorOrderedChannel, selector: "stable",
			resolved: domain.Resolved{Ref: "stable-2026-09-27", Commit: "c-stable-2026-09-27"}, snap: coreReleaseTags("stable-2026-09-27"),
		},
		{
			name: "a dated stable build whose tag is not visible yet is never offered stable-26.5.1", kind: domain.SelectorOrderedChannel, selector: "stable",
			resolved: domain.Resolved{Ref: "stable-2026-09-27", Commit: "sdate"}, snap: coreReleaseTags(),
		},
		{
			name: "a dated beta build is never offered beta-26.5-4", kind: domain.SelectorOrderedChannel, selector: "beta",
			resolved: domain.Resolved{Ref: "beta-2026-09-27", Commit: "bdate"}, snap: coreReleaseTags(),
		},
		{
			name: "stable-26.5.1 is offered the dated release", kind: domain.SelectorOrderedChannel, selector: "stable",
			resolved: domain.Resolved{Ref: "stable-26.5.1", Commit: "s2651"}, snap: coreReleaseTags("stable-2026-09-27"),
			wantTarget: domain.Available{Ref: "stable-2026-09-27", Commit: "c-stable-2026-09-27"}, wantOutdated: true,
		},
		{
			name: "the dated release is offered the next calendar version", kind: domain.SelectorOrderedChannel, selector: "stable",
			resolved: domain.Resolved{Ref: "stable-2026-09-27", Commit: "c-stable-2026-09-27"}, snap: coreReleaseTags("stable-2026-09-27", "stable-26.10"),
			wantTarget: domain.Available{Ref: "stable-26.10", Commit: "c-stable-26.10"}, wantOutdated: true,
		},
		{
			name: "a constraint whose installed tag was deleted is not offered a lower one", kind: domain.SelectorConstraint, selector: "v1.*",
			resolved: domain.Resolved{Ref: "v1.3.0", Commit: "gone"}, snap: domain.RefSnapshot{Tags: map[string]string{"v1.2.0": "c120"}},
		},
		{
			name: "a moved tag of the same name is still offered", kind: domain.SelectorOrderedChannel, selector: "stable",
			resolved: domain.Resolved{Ref: "stable-26.5.1", Commit: "old"}, snap: coreReleaseTags(),
			wantTarget: domain.Available{Ref: "stable-26.5.1", Commit: "s2651"}, wantOutdated: true,
		},
		{
			name: "a rolling tag moved backwards is still followed", kind: domain.SelectorPointerChannel, selector: "nightly-latest",
			resolved: domain.Resolved{Ref: "nightly-latest", Commit: "newer"}, snap: coreReleaseTags(),
			wantTarget: domain.Available{Ref: "nightly-latest", Commit: "n"}, wantOutdated: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			target, outdated, err := Drift(tc.kind, tc.selector, tc.resolved, tc.snap)
			require.NoError(t, err)
			assert.Equal(t, tc.wantOutdated, outdated)
			assert.Equal(t, tc.wantTarget, target)
		})
	}
}

func TestDrift_OrderedChannelRegroupedByLaterTags_FollowsItsInstalledTag(t *testing.T) {
	regrouped := domain.RefSnapshot{Tags: map[string]string{
		"release-1.0": "c10", "release-1.1": "c11", "release-1.2": "c12", "foo-2.0": "c20",
	}}

	testCases := []struct {
		name         string
		kind         domain.SelectorKind
		resolved     domain.Resolved
		wantTarget   domain.Available
		wantOutdated bool
		wantErr      error
	}{
		{
			name: "refined row follows the channel holding its installed tag", kind: domain.SelectorOrderedChannel,
			resolved: domain.Resolved{Ref: "release-1.1", Commit: "c11"}, wantTarget: domain.Available{Ref: "release-1.2", Commit: "c12"}, wantOutdated: true,
		},
		{
			name: "unrefined row does the same", kind: domain.SelectorChannel,
			resolved: domain.Resolved{Ref: "release-1.2", Commit: "c12"},
		},
		{
			name: "an installed tag no channel holds has no answer", kind: domain.SelectorOrderedChannel,
			resolved: domain.Resolved{Ref: "release-0.9", Commit: "c09"}, wantErr: ErrUnknownSelector,
		},
		{
			name: "nothing installed has no answer", kind: domain.SelectorOrderedChannel,
			resolved: domain.Resolved{}, wantErr: ErrUnknownSelector,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			target, outdated, err := Drift(tc.kind, StableChannel, tc.resolved, regrouped)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantOutdated, outdated)
			assert.Equal(t, tc.wantTarget, target)
		})
	}
}

// The names release-tag.sh publishes for a dated series (pinned by
// tests/releasetags): patches count .1, .2 after the date, rebuilds -1, -2.
func TestChannelsOf_DatedSeriesAsTheWorkflowsNameIt(t *testing.T) {
	snap := coreReleaseTags("stable-2026-09-27", "stable-2026-09-27.1", "hotfix-2026-09-27.1", "hotfix-2026-09-27.1-1", "beta-2026-09-27-1")

	byName := map[string]ChannelInfo{}
	for _, c := range ChannelsOf(snap) {
		byName[c.Name] = c
	}

	assert.ElementsMatch(t, []string{"stable", "beta", "hotfix", "nightly-latest"}, keys(byName), "no bogus channel from a date")
	assert.Equal(t, []string{"stable-2026-09-27.1", "stable-2026-09-27", "stable-26.5.1", "stable-26.5"}, byName["stable"].Members)
	assert.Equal(t, []string{"hotfix-2026-09-27.1-1", "hotfix-2026-09-27.1", "hotfix-26.5.2"}, byName["hotfix"].Members)
	assert.Equal(t, "beta-2026-09-27-1", byName["beta"].Latest)
}

func keys(m map[string]ChannelInfo) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// A stable row, the core's own (channel:ordered, adopted at the build's
// main.version) and an unrefined one alike, walks the dated series in order:
// each release offers exactly the next one and never an earlier one.
func TestDrift_DatedStableSeries_OfferedInOrder(t *testing.T) {
	steps := []string{"stable-26.5.1", "stable-2026-09-27", "stable-2026-09-27.1", "stable-2026-10-01", "stable-26.11"}

	for _, kind := range []domain.SelectorKind{domain.SelectorOrderedChannel, domain.SelectorChannel} {
		t.Run(string(kind), func(t *testing.T) {
			for i := 0; i+1 < len(steps); i++ {
				installed, next := steps[i], steps[i+1]
				snap := coreReleaseTags(steps[1 : i+2]...)
				resolved := domain.Resolved{Ref: installed, Commit: snap.Tags[installed]}

				target, outdated, err := Drift(kind, StableChannel, resolved, snap)

				require.NoError(t, err)
				require.True(t, outdated, "%s must be offered %s", installed, next)
				assert.Equal(t, next, target.Ref)
				_, again, err := Drift(kind, StableChannel, domain.Resolved{Ref: next, Commit: target.Commit}, snap)
				require.NoError(t, err)
				assert.False(t, again, "%s is the head", next)
			}
		})
	}
}

func TestTarget_HotfixChannelOfADatedSeries(t *testing.T) {
	snap := coreReleaseTags("stable-2026-09-27", "hotfix-2026-09-27.1")

	kind, err := ClassifySelector("hotfix", snap)
	require.NoError(t, err)
	target, err := Target(kind, "hotfix", snap)

	require.NoError(t, err)
	assert.Equal(t, "hotfix-2026-09-27.1", target.Ref)
}
