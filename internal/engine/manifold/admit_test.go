package manifold

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestAdmit(t *testing.T) {
	headOnly := domain.RefSnapshot{Branches: map[string]string{"main": "cm"}, Head: "main"}
	sameName := domain.RefSnapshot{
		Tags:     map[string]string{"main": "ctag", "v1.0.0": "c100"},
		Branches: map[string]string{"main": "cbranch"},
		Head:     "main",
	}
	commits := domain.RefSnapshot{
		Tags:     map[string]string{"v1.2.0": "abcdef1234567890", "v1.3.0": "fedcba0987654321"},
		Branches: map[string]string{"main": "ABCDEF1234567890"},
	}

	testCases := []struct {
		name     string
		kind     domain.SelectorKind
		selector string
		ref      string
		snap     domain.RefSnapshot
		want     domain.Available
		wantErr  error
	}{
		{
			name: "ordered channel admits its newest member", kind: domain.SelectorChannel, selector: "stable",
			ref: "v2.0.0", snap: releaseSnapshot(), want: domain.Available{Ref: "v2.0.0", Commit: "c3"},
		},
		{
			name: "ordered channel admits an older member", kind: domain.SelectorChannel, selector: "stable",
			ref: "v1.2.0", snap: releaseSnapshot(), want: domain.Available{Ref: "v1.2.0", Commit: "c1"},
		},
		{
			name: "ordered channel refuses a pointer tag", kind: domain.SelectorChannel, selector: "stable",
			ref: "nightly-latest", snap: releaseSnapshot(), wantErr: ErrNotAdmitted,
		},
		{
			name: "ordered channel refuses a branch", kind: domain.SelectorChannel, selector: "stable",
			ref: "main", snap: releaseSnapshot(), wantErr: ErrNotAdmitted,
		},
		{
			name: "pointer channel admits its own tag", kind: domain.SelectorChannel, selector: "nightly-latest",
			ref: "nightly-latest", snap: releaseSnapshot(), want: domain.Available{Ref: "nightly-latest", Commit: "new"},
		},
		{
			name: "pointer channel refuses any other tag", kind: domain.SelectorChannel, selector: "nightly-latest",
			ref: "v2.0.0", snap: releaseSnapshot(), wantErr: ErrNotAdmitted,
		},
		{
			name: "default-branch channel admits the branch", kind: domain.SelectorChannel, selector: "main",
			ref: "main", snap: headOnly, want: domain.Available{Ref: "main", Commit: "cm"},
		},
		{
			name: "a channel the snapshot no longer lists admits nothing", kind: domain.SelectorChannel, selector: "beta",
			ref: "v2.0.0", snap: releaseSnapshot(), wantErr: ErrNotAdmitted,
		},
		{
			name: "constraint admits a matching tag", kind: domain.SelectorConstraint, selector: "v1.*",
			ref: "v1.2.0", snap: releaseSnapshot(), want: domain.Available{Ref: "v1.2.0", Commit: "c1"},
		},
		{
			name: "constraint refuses a tag outside the glob", kind: domain.SelectorConstraint, selector: "v1.*",
			ref: "v2.0.0", snap: releaseSnapshot(), wantErr: ErrNotAdmitted,
		},
		{
			name: "constraint refuses a branch even when it matches", kind: domain.SelectorConstraint, selector: "ma*",
			ref: "main", snap: releaseSnapshot(), wantErr: ErrNotAdmitted,
		},
		{
			name: "constraint with a malformed glob admits nothing", kind: domain.SelectorConstraint, selector: "v1.[",
			ref: "v1.2.0", snap: releaseSnapshot(), wantErr: ErrNotAdmitted,
		},
		{
			name: "pin admits its own ref", kind: domain.SelectorPin, selector: "v1.2.0",
			ref: "v1.2.0", snap: releaseSnapshot(), want: domain.Available{Ref: "v1.2.0", Commit: "c1"},
		},
		{
			name: "pin refuses a different ref", kind: domain.SelectorPin, selector: "v1.2.0",
			ref: "v1.3.0", snap: releaseSnapshot(), wantErr: ErrNotAdmitted,
		},
		{
			name: "escaped branch pin takes the branch commit over a same-name tag", kind: domain.SelectorPin,
			selector: "refs/heads/main", ref: "main", snap: sameName,
			want: domain.Available{Ref: "main", Commit: "cbranch"},
		},
		{
			name: "pin whose ref is gone admits nothing", kind: domain.SelectorPin, selector: "v9.9.9",
			ref: "v1.2.0", snap: releaseSnapshot(), wantErr: ErrNotAdmitted,
		},
		{
			name: "commit selector admits itself", kind: domain.SelectorCommit, selector: "abcdef1",
			ref: "abcdef1", snap: commits, want: domain.Available{Ref: "abcdef1", Commit: "abcdef1"},
		},
		{
			name: "commit selector admits a ref at a commit it prefixes", kind: domain.SelectorCommit, selector: "abcdef1",
			ref: "v1.2.0", snap: commits, want: domain.Available{Ref: "v1.2.0", Commit: "abcdef1234567890"},
		},
		{
			name: "commit selector matches regardless of case", kind: domain.SelectorCommit, selector: "abcdef1",
			ref: "main", snap: commits, want: domain.Available{Ref: "main", Commit: "ABCDEF1234567890"},
		},
		{
			name: "uppercase commit selector admits itself spelled in lowercase", kind: domain.SelectorCommit, selector: "ABCDEF1",
			ref: "abcdef1", snap: commits, want: domain.Available{Ref: "ABCDEF1", Commit: "ABCDEF1"},
		},
		{
			name: "commit selector spelled differently records the row an add would", kind: domain.SelectorCommit, selector: "abcdef1",
			ref: "ABCDEF1", snap: commits, want: domain.Available{Ref: "abcdef1", Commit: "abcdef1"},
		},
		{
			name: "commit selector refuses a ref at another commit", kind: domain.SelectorCommit, selector: "abcdef1",
			ref: "v1.3.0", snap: commits, wantErr: ErrNotAdmitted,
		},
		{
			name: "a ref the snapshot does not hold is unknown", kind: domain.SelectorChannel, selector: "stable",
			ref: "v0.0.1", snap: releaseSnapshot(), wantErr: ErrUnknownSelector,
		},
		{
			name: "an unknown kind admits nothing", kind: domain.SelectorKind("bogus"), selector: "stable",
			ref: "v1.2.0", snap: releaseSnapshot(), wantErr: ErrNotAdmitted,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Admit(tc.kind, tc.selector, tc.ref, tc.snap)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestAdmit_TheInstallTargetIsAlwaysAdmitted(t *testing.T) {
	testCases := []struct {
		name     string
		kind     domain.SelectorKind
		selector string
	}{
		{name: "ordered channel", kind: domain.SelectorChannel, selector: "stable"},
		{name: "pointer channel", kind: domain.SelectorChannel, selector: "nightly-latest"},
		{name: "constraint", kind: domain.SelectorConstraint, selector: "v1.*"},
		{name: "pin", kind: domain.SelectorPin, selector: "v1.3.0"},
		{name: "escaped pin", kind: domain.SelectorPin, selector: "refs/heads/main"},
		{name: "commit", kind: domain.SelectorCommit, selector: "c3c3c3c"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			target, err := Target(tc.kind, tc.selector, releaseSnapshot())
			require.NoError(t, err)

			got, err := Admit(tc.kind, tc.selector, target.Ref, releaseSnapshot())
			require.NoError(t, err)
			assert.Equal(t, target, got)
		})
	}
}
