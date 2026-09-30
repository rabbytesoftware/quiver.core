package admit

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
