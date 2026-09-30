package manifold

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

const (
	fullSHA  = "9dd0b183177a64ec71a2672d1cd7cf0c70bb4877"
	shortSHA = "9dd0b18"
)

func sharedSnapshot() domain.RefSnapshot {
	return domain.RefSnapshot{
		Tags: map[string]string{
			"v1.2.0":         "c-v1.2.0",
			"v1.3.0":         "c-v1.3.0",
			"beta-26.5-4":    "c-beta-4",
			"beta-26.5-1":    "c-beta-1",
			"stable-26.5.1":  "c-stable-26.5.1",
			"nightly":        "c-nightly",
			"nightly-latest": "c-nightly-latest",
			"v1.0-latest":    "c-v1.0-latest",
		},
		Branches: map[string]string{"develop": "c-develop", "deadbeef": "c-deadbeef"},
		Head:     "develop",
	}
}

func withTag(
	snap domain.RefSnapshot,
	tag string,
	commit string,
) domain.RefSnapshot {
	tags := make(map[string]string, len(snap.Tags)+1)
	for k, v := range snap.Tags {
		tags[k] = v
	}
	tags[tag] = commit
	snap.Tags = tags
	return snap
}

func withBranch(
	snap domain.RefSnapshot,
	branch string,
) domain.RefSnapshot {
	branches := make(map[string]string, len(snap.Branches)+1)
	for k, v := range snap.Branches {
		branches[k] = v
	}
	branches[branch] = "c-" + branch
	snap.Branches = branches
	return snap
}

func TestClassifySelector(t *testing.T) {
	testCases := []struct {
		name     string
		selector string
		snap     domain.RefSnapshot
		want     domain.SelectorKind
		wantErr  error
	}{
		{name: "ordered channel stable", selector: "stable", snap: sharedSnapshot(), want: domain.SelectorOrderedChannel},
		{name: "ordered channel beta", selector: "beta", snap: sharedSnapshot(), want: domain.SelectorOrderedChannel},
		{name: "pointer channel nightly", selector: "nightly", snap: sharedSnapshot(), want: domain.SelectorPointerChannel},
		{name: "pointer channel nightly-latest", selector: "nightly-latest", snap: sharedSnapshot(), want: domain.SelectorPointerChannel},
		{name: "exact tag is a pin", selector: "v1.2.0", snap: sharedSnapshot(), want: domain.SelectorTagPin},
		{name: "exact branch is a pin", selector: "develop", snap: sharedSnapshot(), want: domain.SelectorBranchPin},
		{name: "glob is a constraint", selector: "v1.*", snap: sharedSnapshot(), want: domain.SelectorConstraint},
		{name: "short hex naming no ref is a commit", selector: shortSHA, snap: sharedSnapshot(), want: domain.SelectorCommit},
		{name: "full hex naming no ref is a commit", selector: fullSHA, snap: sharedSnapshot(), want: domain.SelectorCommit},
		{name: "uppercase hex is a commit", selector: "9DD0B18", snap: sharedSnapshot(), want: domain.SelectorCommit},
		{name: "hex naming a branch is a pin", selector: "deadbeef", snap: sharedSnapshot(), want: domain.SelectorBranchPin},
		{name: "unknown name", selector: "nope", snap: sharedSnapshot(), wantErr: ErrUnknownSelector},
		{name: "hex too short", selector: "9dd0b1", snap: sharedSnapshot(), wantErr: ErrUnknownSelector},
		{name: "hex too long", selector: fullSHA + "0", snap: sharedSnapshot(), wantErr: ErrUnknownSelector},
		{name: "empty selector", selector: "", snap: sharedSnapshot(), wantErr: ErrUnknownSelector},
		{name: "malformed glob", selector: "v1.[", snap: sharedSnapshot(), wantErr: ErrUnknownSelector},
		{name: "tag named stable does not shadow the channel", selector: "stable", snap: withTag(sharedSnapshot(), "stable", "c-stable-tag"), want: domain.SelectorOrderedChannel},
		{name: "escaped tag is a pin", selector: "refs/tags/stable", snap: withTag(sharedSnapshot(), "stable", "c-stable-tag"), want: domain.SelectorTagPin},
		{name: "escaped branch is a pin", selector: "refs/heads/develop", snap: sharedSnapshot(), want: domain.SelectorBranchPin},
		{name: "escaped missing tag", selector: "refs/tags/stable", snap: sharedSnapshot(), wantErr: ErrUnknownSelector},
		{name: "escaped missing branch", selector: "refs/heads/nope", snap: sharedSnapshot(), wantErr: ErrUnknownSelector},
		{name: "doubled slash names nothing", selector: "feat//x", snap: withBranch(sharedSnapshot(), "feat//x"), wantErr: ErrUnknownSelector},
		{name: "trailing slash names nothing", selector: "feat/", snap: withBranch(sharedSnapshot(), "feat/"), wantErr: ErrUnknownSelector},
		{name: "leading slash names nothing", selector: "/feat", snap: withBranch(sharedSnapshot(), "/feat"), wantErr: ErrUnknownSelector},
		{name: "escape with no name names nothing", selector: "refs/heads/", snap: withBranch(sharedSnapshot(), ""), wantErr: ErrUnknownSelector},
		{name: "nested branch is a pin", selector: "feat/x", snap: withBranch(sharedSnapshot(), "feat/x"), want: domain.SelectorBranchPin},
		{name: "exact tag shadowing a same-name branch is a tag pin", selector: "v1.2.0", snap: withBranch(sharedSnapshot(), "v1.2.0"), want: domain.SelectorTagPin},
		{name: "tagless repo head branch is a channel", selector: "develop", snap: domain.RefSnapshot{Branches: map[string]string{"develop": "c"}, Head: "develop"}, want: domain.SelectorBranchChannel},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ClassifySelector(tc.selector, tc.snap)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDefaultChannel(t *testing.T) {
	testCases := []struct {
		name    string
		snap    domain.RefSnapshot
		want    string
		wantErr error
	}{
		{
			name: "stable when an ordered stable member exists",
			snap: sharedSnapshot(),
			want: StableChannel,
		},
		{
			name: "first listed channel when there is no stable member",
			snap: domain.RefSnapshot{Tags: map[string]string{"v1.0.0-rc1": "a", "nightly": "b"}},
			want: "rc",
		},
		{
			name: "the newest channel over an alphabetically earlier one",
			snap: domain.RefSnapshot{Tags: map[string]string{"v0.0.4-alpha.1": "a", "v0.0.44-beta.3": "b"}},
			want: "beta",
		},
		{
			name: "first pointer channel when only pointer tags exist",
			snap: domain.RefSnapshot{Tags: map[string]string{"nightly": "b", "edge": "c"}, Branches: map[string]string{"main": "d"}, Head: "main"},
			want: "edge",
		},
		{
			name: "head branch when the repo has no tags",
			snap: domain.RefSnapshot{Branches: map[string]string{"main": "d"}, Head: "main"},
			want: "main",
		},
		{
			name:    "nothing to follow",
			snap:    domain.RefSnapshot{Branches: map[string]string{"main": "d"}},
			wantErr: ErrUnknownSelector,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DefaultChannel(tc.snap)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
