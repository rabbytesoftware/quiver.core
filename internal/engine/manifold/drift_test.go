package manifold

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func releaseSnapshot() domain.RefSnapshot {
	return domain.RefSnapshot{
		Tags: map[string]string{
			"v1.2.0":         "c1",
			"v1.3.0":         "c2",
			"v2.0.0":         "c3",
			"nightly-latest": "new",
			"v1.0-latest":    "rolled",
		},
		Branches: map[string]string{"main": "cm"},
		Head:     "main",
	}
}

func TestDrift(t *testing.T) {
	stableOnly := domain.RefSnapshot{Tags: map[string]string{"v1.2.0": "c1", "v1.3.0": "c2"}}

	testCases := []struct {
		name         string
		kind         domain.SelectorKind
		selector     string
		resolved     domain.Resolved
		snap         domain.RefSnapshot
		wantTarget   domain.Available
		wantOutdated bool
		wantErr      error
	}{
		{
			name: "channel stable: newer ref is outdated", kind: domain.SelectorChannel, selector: "stable",
			resolved: domain.Resolved{Ref: "v1.2.0", Commit: "c1"}, snap: stableOnly,
			wantTarget: domain.Available{Ref: "v1.3.0", Commit: "c2"}, wantOutdated: true,
		},
		{
			name: "channel stable: at latest is current", kind: domain.SelectorChannel, selector: "stable",
			resolved: domain.Resolved{Ref: "v1.3.0", Commit: "c2"}, snap: stableOnly,
		},
		{
			name: "pointer channel: moved commit is outdated", kind: domain.SelectorChannel, selector: "nightly-latest",
			resolved: domain.Resolved{Ref: "nightly-latest", Commit: "old"}, snap: releaseSnapshot(),
			wantTarget: domain.Available{Ref: "nightly-latest", Commit: "new"}, wantOutdated: true,
		},
		{
			name: "pointer channel: same commit is current", kind: domain.SelectorChannel, selector: "nightly-latest",
			resolved: domain.Resolved{Ref: "nightly-latest", Commit: "new"}, snap: releaseSnapshot(),
		},
		{
			name: "pointer channel: empty resolved is outdated", kind: domain.SelectorChannel, selector: "nightly-latest",
			resolved: domain.Resolved{}, snap: releaseSnapshot(),
			wantTarget: domain.Available{Ref: "nightly-latest", Commit: "new"}, wantOutdated: true,
		},
		{
			name: "rolling tag with a numeric core pinned: moved commit is outdated", kind: domain.SelectorPin, selector: "v1.0-latest",
			resolved: domain.Resolved{Ref: "v1.0-latest", Commit: "old"}, snap: releaseSnapshot(),
			wantTarget: domain.Available{Ref: "v1.0-latest", Commit: "rolled"}, wantOutdated: true,
		},
		{
			name: "rolling tag with a numeric core as a channel: moved commit is outdated", kind: domain.SelectorChannel, selector: "latest",
			resolved: domain.Resolved{Ref: "v1.0-latest", Commit: "old"}, snap: releaseSnapshot(),
			wantTarget: domain.Available{Ref: "v1.0-latest", Commit: "rolled"}, wantOutdated: true,
		},
		{
			name: "rolling tag with a numeric core: unmoved is current", kind: domain.SelectorPin, selector: "v1.0-latest",
			resolved: domain.Resolved{Ref: "v1.0-latest", Commit: "rolled"}, snap: releaseSnapshot(),
		},
		{
			name: "constraint: highest match without crossing majors", kind: domain.SelectorConstraint, selector: "v1.*",
			resolved: domain.Resolved{Ref: "v1.2.0", Commit: "c1"}, snap: releaseSnapshot(),
			wantTarget: domain.Available{Ref: "v1.3.0", Commit: "c2"}, wantOutdated: true,
		},
		{
			name: "constraint: at highest match is current", kind: domain.SelectorConstraint, selector: "v1.*",
			resolved: domain.Resolved{Ref: "v1.3.0", Commit: "c2"}, snap: releaseSnapshot(),
		},
		{
			name: "constraint: moved commit on the same ref is outdated", kind: domain.SelectorConstraint, selector: "v1.*",
			resolved: domain.Resolved{Ref: "v1.3.0", Commit: "old"}, snap: releaseSnapshot(),
			wantTarget: domain.Available{Ref: "v1.3.0", Commit: "c2"}, wantOutdated: true,
		},
		{
			name: "pin: force-moved ordered tag is outdated", kind: domain.SelectorPin, selector: "v1.2.0",
			resolved: domain.Resolved{Ref: "v1.2.0", Commit: "old"}, snap: releaseSnapshot(),
			wantTarget: domain.Available{Ref: "v1.2.0", Commit: "c1"}, wantOutdated: true,
		},
		{
			name: "pin: unmoved is current", kind: domain.SelectorPin, selector: "v1.2.0",
			resolved: domain.Resolved{Ref: "v1.2.0", Commit: "c1"}, snap: releaseSnapshot(),
		},
		{
			name: "pin: branch that moved is outdated", kind: domain.SelectorPin, selector: "main",
			resolved: domain.Resolved{Ref: "main", Commit: "old"}, snap: releaseSnapshot(),
			wantTarget: domain.Available{Ref: "main", Commit: "cm"}, wantOutdated: true,
		},
		{
			name: "pin: escaped tag targets its short name", kind: domain.SelectorPin, selector: "refs/tags/v1.2.0",
			resolved: domain.Resolved{Ref: "v1.2.0", Commit: "c1"}, snap: releaseSnapshot(),
		},
		{
			name: "commit: never outdated", kind: domain.SelectorCommit, selector: fullSHA,
			resolved: domain.Resolved{Ref: fullSHA, Commit: "whatever"}, snap: releaseSnapshot(),
		},
		{
			name: "commit: never outdated even when unresolved", kind: domain.SelectorCommit, selector: fullSHA,
			snap: domain.RefSnapshot{},
		},
		{
			name: "channel absent from snapshot", kind: domain.SelectorChannel, selector: "beta",
			resolved: domain.Resolved{Ref: "v1.2.0", Commit: "c1"}, snap: releaseSnapshot(), wantErr: ErrUnknownSelector,
		},
		{
			name: "constraint matching nothing", kind: domain.SelectorConstraint, selector: "v9.*",
			resolved: domain.Resolved{Ref: "v1.2.0", Commit: "c1"}, snap: releaseSnapshot(), wantErr: ErrUnknownSelector,
		},
		{
			name: "malformed constraint", kind: domain.SelectorConstraint, selector: "v1.[",
			resolved: domain.Resolved{Ref: "v1.2.0", Commit: "c1"}, snap: releaseSnapshot(), wantErr: ErrUnknownSelector,
		},
		{
			name: "pinned ref deleted", kind: domain.SelectorPin, selector: "v0.9.0",
			resolved: domain.Resolved{Ref: "v0.9.0", Commit: "c0"}, snap: releaseSnapshot(), wantErr: ErrUnknownSelector,
		},
		{
			name: "invalid kind", kind: domain.SelectorKind("bogus"), selector: "v1.2.0",
			resolved: domain.Resolved{Ref: "v1.2.0", Commit: "c1"}, snap: releaseSnapshot(), wantErr: ErrUnknownSelector,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			target, outdated, err := Drift(tc.kind, tc.selector, tc.resolved, tc.snap)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				assert.False(t, outdated)
				assert.Equal(t, domain.Available{}, target)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantOutdated, outdated)
			assert.Equal(t, tc.wantTarget, target)
		})
	}
}

func TestTarget(t *testing.T) {
	stableCollision := withTag(domain.RefSnapshot{Tags: map[string]string{"v1.3.0": "c2"}}, "stable", "c-stable-tag")

	testCases := []struct {
		name     string
		kind     domain.SelectorKind
		selector string
		snap     domain.RefSnapshot
		want     domain.Available
		wantErr  error
	}{
		{name: "commit targets itself", kind: domain.SelectorCommit, selector: shortSHA, snap: domain.RefSnapshot{}, want: domain.Available{Ref: shortSHA, Commit: shortSHA}},
		{name: "ordered channel wins over a tag of the same name", kind: domain.SelectorChannel, selector: "stable", snap: stableCollision, want: domain.Available{Ref: "v1.3.0", Commit: "c2"}},
		{name: "escaped tag reaches the shadowed tag", kind: domain.SelectorPin, selector: "refs/tags/stable", snap: stableCollision, want: domain.Available{Ref: "stable", Commit: "c-stable-tag"}},
		{name: "escaped branch", kind: domain.SelectorPin, selector: "refs/heads/main", snap: releaseSnapshot(), want: domain.Available{Ref: "main", Commit: "cm"}},
		{name: "default branch fallback channel", kind: domain.SelectorChannel, selector: "main", snap: domain.RefSnapshot{Branches: map[string]string{"main": "cm"}, Head: "main"}, want: domain.Available{Ref: "main", Commit: "cm"}},
		{name: "channel whose latest has no commit", kind: domain.SelectorChannel, selector: "main", snap: domain.RefSnapshot{Head: "main"}, wantErr: ErrUnknownSelector},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Target(tc.kind, tc.selector, tc.snap)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
