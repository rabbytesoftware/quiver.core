package selector

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
