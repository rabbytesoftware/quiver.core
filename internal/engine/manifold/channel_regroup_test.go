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
