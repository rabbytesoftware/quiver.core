package manifold

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// laterPushes is a repository after tags and branches sharing names with
// earlier refs were pushed: a dated build next to the rolling "nightly", a
// "develop" tag next to the branch, a "master" tag next to the branch, and a
// "stable" tag next to the ordered channel.
func laterPushes() domain.RefSnapshot {
	return domain.RefSnapshot{
		Tags: map[string]string{
			"v1.2.0": "c120", "v1.3.0": "c130",
			"nightly": "rolling", "nightly-2026.09.30": "dated",
			"develop": "develop-tag", "master": "master-tag", "stable": "stable-tag",
		},
		Branches: map[string]string{"develop": "develop-branch", "master": "master-branch", "main": "main-branch"},
		Head:     "main",
	}
}

func TestTarget_RefinedKindReadsOnlyItsOwnRef(t *testing.T) {
	testCases := []struct {
		name     string
		kind     domain.SelectorKind
		selector string
		want     domain.Available
		wantErr  error
	}{
		{name: "pointer channel keeps its rolling tag", kind: domain.SelectorPointerChannel, selector: "nightly", want: domain.Available{Ref: "nightly", Commit: "rolling"}},
		{name: "legacy channel prefers the ordered channel", kind: domain.SelectorChannel, selector: "nightly", want: domain.Available{Ref: "nightly-2026.09.30", Commit: "dated"}},
		{name: "ordered channel ignores a same-name tag", kind: domain.SelectorOrderedChannel, selector: "stable", want: domain.Available{Ref: "v1.3.0", Commit: "c130"}},
		{name: "ordered channel absent", kind: domain.SelectorOrderedChannel, selector: "beta", wantErr: ErrUnknownSelector},
		{name: "branch pin keeps its branch", kind: domain.SelectorBranchPin, selector: "develop", want: domain.Available{Ref: "develop", Commit: "develop-branch"}},
		{name: "tag pin keeps its tag", kind: domain.SelectorTagPin, selector: "develop", want: domain.Available{Ref: "develop", Commit: "develop-tag"}},
		{name: "legacy pin prefers the tag", kind: domain.SelectorPin, selector: "develop", want: domain.Available{Ref: "develop", Commit: "develop-tag"}},
		{name: "escaped branch pin keeps its branch", kind: domain.SelectorPin, selector: "refs/heads/master", want: domain.Available{Ref: "master", Commit: "master-branch"}},
		{name: "branch channel keeps its branch", kind: domain.SelectorBranchChannel, selector: "main", want: domain.Available{Ref: "main", Commit: "main-branch"}},
		{name: "branch channel whose branch is gone", kind: domain.SelectorBranchChannel, selector: "trunk", wantErr: ErrUnknownSelector},
		{name: "tag pin whose tag is gone never falls to a branch", kind: domain.SelectorTagPin, selector: "main", wantErr: ErrUnknownSelector},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Target(tc.kind, tc.selector, laterPushes())
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRefCommit_LooksUpWhereTheRowsRefsLive(t *testing.T) {
	testCases := []struct {
		name     string
		kind     domain.SelectorKind
		selector string
		ref      string
		want     string
		wantOK   bool
	}{
		{name: "escaped branch pin reads the branch", kind: domain.SelectorBranchPin, selector: "refs/heads/master", ref: "master", want: "master-branch", wantOK: true},
		{name: "legacy escaped branch pin reads the branch", kind: domain.SelectorPin, selector: "refs/heads/master", ref: "master", want: "master-branch", wantOK: true},
		{name: "branch pin reads the branch", kind: domain.SelectorBranchPin, selector: "develop", ref: "develop", want: "develop-branch", wantOK: true},
		{name: "tag pin reads the tag", kind: domain.SelectorTagPin, selector: "develop", ref: "develop", want: "develop-tag", wantOK: true},
		{name: "legacy pin prefers the tag", kind: domain.SelectorPin, selector: "develop", ref: "develop", want: "develop-tag", wantOK: true},
		{name: "ordered channel reads tags", kind: domain.SelectorOrderedChannel, selector: "stable", ref: "v1.3.0", want: "c130", wantOK: true},
		{name: "ordered channel never reads a branch", kind: domain.SelectorOrderedChannel, selector: "stable", ref: "main", wantOK: false},
		{name: "constraint reads tags", kind: domain.SelectorConstraint, selector: "v1.*", ref: "v1.2.0", want: "c120", wantOK: true},
		{name: "pointer channel reads its tag", kind: domain.SelectorPointerChannel, selector: "nightly", ref: "nightly", want: "rolling", wantOK: true},
		{name: "branch channel reads its branch", kind: domain.SelectorBranchChannel, selector: "main", ref: "main", want: "main-branch", wantOK: true},
		{name: "legacy channel prefers a tag", kind: domain.SelectorChannel, selector: "stable", ref: "v1.2.0", want: "c120", wantOK: true},
		{name: "commit naming no ref is itself", kind: domain.SelectorCommit, selector: "abcdef1", ref: "abcdef1", want: "abcdef1", wantOK: true},
		{name: "commit row reading a ref", kind: domain.SelectorCommit, selector: "abcdef1", ref: "v1.2.0", want: "c120", wantOK: true},
		{name: "missing ref", kind: domain.SelectorTagPin, selector: "gone", ref: "gone", wantOK: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := RefCommit(tc.kind, tc.selector, tc.ref, laterPushes())
			assert.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestAdmit_RefinedKinds(t *testing.T) {
	testCases := []struct {
		name     string
		kind     domain.SelectorKind
		selector string
		ref      string
		want     domain.Available
		wantErr  error
	}{
		{name: "branch pin admits its branch at the branch's commit", kind: domain.SelectorBranchPin, selector: "develop", ref: "develop", want: domain.Available{Ref: "develop", Commit: "develop-branch"}},
		{name: "tag pin admits its tag", kind: domain.SelectorTagPin, selector: "v1.2.0", ref: "v1.2.0", want: domain.Available{Ref: "v1.2.0", Commit: "c120"}},
		{name: "tag pin refuses another tag", kind: domain.SelectorTagPin, selector: "v1.2.0", ref: "v1.3.0", wantErr: ErrNotAdmitted},
		{name: "pointer channel admits its own tag", kind: domain.SelectorPointerChannel, selector: "nightly", ref: "nightly", want: domain.Available{Ref: "nightly", Commit: "rolling"}},
		{name: "pointer channel refuses the dated build", kind: domain.SelectorPointerChannel, selector: "nightly", ref: "nightly-2026.09.30", wantErr: ErrNotAdmitted},
		{name: "branch channel admits its branch", kind: domain.SelectorBranchChannel, selector: "main", ref: "main", want: domain.Available{Ref: "main", Commit: "main-branch"}},
		{name: "ordered channel admits a member", kind: domain.SelectorOrderedChannel, selector: "stable", ref: "v1.2.0", want: domain.Available{Ref: "v1.2.0", Commit: "c120"}},
		{name: "ordered channel refuses the same-name tag", kind: domain.SelectorOrderedChannel, selector: "stable", ref: "stable", wantErr: ErrNotAdmitted},
		{name: "legacy channel admits the head branch it settled on", kind: domain.SelectorChannel, selector: "main", ref: "main", want: domain.Available{Ref: "main", Commit: "main-branch"}},
		{name: "legacy channel refuses another branch", kind: domain.SelectorChannel, selector: "trunk", ref: "develop", wantErr: ErrNotAdmitted},
		{name: "unknown kind admits nothing", kind: domain.SelectorKind("floating"), selector: "v1.2.0", ref: "v1.2.0", wantErr: ErrNotAdmitted},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Admit(tc.kind, tc.selector, tc.ref, laterPushes())
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
