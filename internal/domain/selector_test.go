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
		{name: "unknown", kind: SelectorKind("floating"), want: false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) { assert.Equal(t, tc.want, tc.kind.Valid()) })
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
