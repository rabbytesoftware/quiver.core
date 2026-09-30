package drift

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	sel "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/versioning/internal/selector"
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
		{name: "ordered channel absent", kind: domain.SelectorOrderedChannel, selector: "beta", wantErr: sel.ErrUnknownSelector},
		{name: "branch pin keeps its branch", kind: domain.SelectorBranchPin, selector: "develop", want: domain.Available{Ref: "develop", Commit: "develop-branch"}},
		{name: "tag pin keeps its tag", kind: domain.SelectorTagPin, selector: "develop", want: domain.Available{Ref: "develop", Commit: "develop-tag"}},
		{name: "legacy pin prefers the tag", kind: domain.SelectorPin, selector: "develop", want: domain.Available{Ref: "develop", Commit: "develop-tag"}},
		{name: "escaped branch pin keeps its branch", kind: domain.SelectorPin, selector: "refs/heads/master", want: domain.Available{Ref: "master", Commit: "master-branch"}},
		{name: "branch channel keeps its branch", kind: domain.SelectorBranchChannel, selector: "main", want: domain.Available{Ref: "main", Commit: "main-branch"}},
		{name: "branch channel whose branch is gone", kind: domain.SelectorBranchChannel, selector: "trunk", wantErr: sel.ErrUnknownSelector},
		{name: "tag pin whose tag is gone never falls to a branch", kind: domain.SelectorTagPin, selector: "main", wantErr: sel.ErrUnknownSelector},
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
