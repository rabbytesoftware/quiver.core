package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectorKind_Valid(t *testing.T) {
	testCases := []struct {
		name string
		kind SelectorKind
		want bool
	}{
		{name: "zero value is a pin", kind: SelectorPin, want: true},
		{name: "channel", kind: SelectorChannel, want: true},
		{name: "constraint", kind: SelectorConstraint, want: true},
		{name: "commit", kind: SelectorCommit, want: true},
		{name: "tag pin", kind: SelectorTagPin, want: true},
		{name: "branch pin", kind: SelectorBranchPin, want: true},
		{name: "ordered channel", kind: SelectorOrderedChannel, want: true},
		{name: "pointer channel", kind: SelectorPointerChannel, want: true},
		{name: "branch channel", kind: SelectorBranchChannel, want: true},
		{name: "unknown", kind: SelectorKind("floating"), want: false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { assert.Equal(t, tc.want, tc.kind.Valid()) })
	}
}

func TestSelectorKind_Family(t *testing.T) {
	testCases := []struct {
		name string
		kind SelectorKind
		want SelectorKind
	}{
		{name: "legacy pin", kind: SelectorPin, want: SelectorPin},
		{name: "tag pin", kind: SelectorTagPin, want: SelectorPin},
		{name: "branch pin", kind: SelectorBranchPin, want: SelectorPin},
		{name: "legacy channel", kind: SelectorChannel, want: SelectorChannel},
		{name: "ordered channel", kind: SelectorOrderedChannel, want: SelectorChannel},
		{name: "pointer channel", kind: SelectorPointerChannel, want: SelectorChannel},
		{name: "branch channel", kind: SelectorBranchChannel, want: SelectorChannel},
		{name: "constraint", kind: SelectorConstraint, want: SelectorConstraint},
		{name: "commit", kind: SelectorCommit, want: SelectorCommit},
		{name: "unknown stays itself", kind: SelectorKind("floating"), want: SelectorKind("floating")},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { assert.Equal(t, tc.want, tc.kind.Family()) })
	}
}

func TestRefSnapshot_Commit_PrefersTagOverBranch(t *testing.T) {
	snap := RefSnapshot{
		Tags:     map[string]string{"nightly": "aaa"},
		Branches: map[string]string{"nightly": "bbb", "develop": "ccc"},
	}
	got, ok := snap.Commit("nightly")
	require.True(t, ok)
	assert.Equal(t, "aaa", got)
	got, ok = snap.Commit("develop")
	require.True(t, ok)
	assert.Equal(t, "ccc", got)
	_, ok = snap.Commit("missing")
	assert.False(t, ok)
}

func TestArrow_ZeroValueRowIsAPinWithNoResolvedState(t *testing.T) {
	var a Arrow
	assert.Equal(t, SelectorPin, a.SelectorKind)
	assert.Empty(t, a.Resolved.Commit)
	assert.Nil(t, a.Available)
}

func TestResolved_RefOr(t *testing.T) {
	testCases := []struct {
		name     string
		resolved Resolved
		fallback string
		want     string
	}{
		{name: "resolved ref wins", resolved: Resolved{Ref: "v1.2.0", Commit: "abc"}, fallback: "stable", want: "v1.2.0"},
		{name: "no resolved ref falls back", resolved: Resolved{}, fallback: "v1.0.0", want: "v1.0.0"},
		{name: "commit alone does not name a ref", resolved: Resolved{Commit: "abc"}, fallback: "main", want: "main"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { assert.Equal(t, tc.want, tc.resolved.RefOr(tc.fallback)) })
	}
}
