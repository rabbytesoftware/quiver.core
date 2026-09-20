package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArrowState_IsActive_RunningReturnsTrue(t *testing.T) {
	assert.True(t, ArrowStateRunning.IsActive())
}

func TestArrowState_IsActive_InstallingReturnsTrue(t *testing.T) {
	assert.True(t, ArrowStateInstalling.IsActive())
}

func TestArrowState_IsActive_UpdatingReturnsTrue(t *testing.T) {
	assert.True(t, ArrowStateUpdating.IsActive())
}

func TestArrowState_IsActive_ReadyReturnsFalse(t *testing.T) {
	assert.False(t, ArrowStateReady.IsActive())
}

func TestArrowState_IsActive_AbsentReturnsFalse(t *testing.T) {
	assert.False(t, ArrowStateAbsent.IsActive())
}

func TestArrowState_IsActive_DetachedReturnsFalse(t *testing.T) {
	assert.False(t, ArrowStateDetached.IsActive())
}

func TestArrowState_CanTransitionTo_ValidTransitions(t *testing.T) {
	valid := []struct {
		from ArrowState
		to   ArrowState
	}{
		{ArrowStateAbsent, ArrowStateReady},
		{ArrowStateReady, ArrowStateRunning},
		{ArrowStateReady, ArrowStateInstalling},
		{ArrowStateReady, ArrowStateUninstalling},
		{ArrowStateReady, ArrowStateUpdating},
		{ArrowStateRunning, ArrowStateStopping},
		{ArrowStateRunning, ArrowStateDetached},
		{ArrowStateStopping, ArrowStateReady},
		{ArrowStateDetached, ArrowStateReady},
		{ArrowStateDetached, ArrowStateRunning},
		{ArrowStateInstalling, ArrowStateReady},
		{ArrowStateInstalling, ArrowStateAbsent},
		{ArrowStateUninstalling, ArrowStateAbsent},
		{ArrowStateUninstalling, ArrowStateReady},
		{ArrowStateUpdating, ArrowStateReady},
		{ArrowStateUpdating, ArrowStateAbsent},
		{ArrowStateReady, ArrowStateOutdated},
		{ArrowStateOutdated, ArrowStateReady},
		{ArrowStateOutdated, ArrowStateUninstalling},
		{ArrowStateOutdated, ArrowStateRunning},
	}

	for _, tt := range valid {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			assert.True(t, tt.from.CanTransitionTo(tt.to))
		})
	}
}

func TestArrowState_CanTransitionTo_InvalidTransitions(t *testing.T) {
	invalid := []struct {
		from ArrowState
		to   ArrowState
	}{
		{ArrowStateAbsent, ArrowStateRunning},
		{ArrowStateAbsent, ArrowStateInstalling},
		{ArrowStateAbsent, ArrowStateAbsent},
		{ArrowStateReady, ArrowStateReady},
		{ArrowStateReady, ArrowStateAbsent},
		{ArrowStateReady, ArrowStateStopping},
		{ArrowStateRunning, ArrowStateReady},
		{ArrowStateRunning, ArrowStateRunning},
		{ArrowStateRunning, ArrowStateAbsent},
		{ArrowStateStopping, ArrowStateRunning},
		{ArrowStateStopping, ArrowStateAbsent},
		{ArrowStateDetached, ArrowStateAbsent},
		{ArrowStateInstalling, ArrowStateRunning},
		{ArrowStateUninstalling, ArrowStateRunning},
		{ArrowStateUpdating, ArrowStateRunning},
	}

	for _, tt := range invalid {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			assert.False(t, tt.from.CanTransitionTo(tt.to))
		})
	}
}

func TestArrowState_CanTransitionTo_TerminalStateRejectsAll(t *testing.T) {
	all := []ArrowState{
		ArrowStateAbsent,
		ArrowStateReady,
		ArrowStateRunning,
		ArrowStateStopping,
		ArrowStateDetached,
		ArrowStateInstalling,
		ArrowStateUninstalling,
		ArrowStateUpdating,
		ArrowStateRemoved,
	}

	for _, target := range all {
		t.Run("removed->"+string(target), func(t *testing.T) {
			assert.False(t, ArrowStateRemoved.CanTransitionTo(target))
		})
	}
}

func TestArrowState_CanTransitionTo_UnknownStateRejectsAll(t *testing.T) {
	unknown := ArrowState("unknown")
	assert.False(t, unknown.CanTransitionTo(ArrowStateReady))
	assert.False(t, ArrowStateReady.CanTransitionTo(unknown))
}

// The ref a manifest was resolved at, and the ref an _install put on disk, are
// both the namespace's own ref on every path — the aggregate is keyed by
// namespace@ref, so no other ref can reach it. A second field restating that ref
// is a place for the two answers to drift apart, so neither may exist.
func TestArrow_HasNoFieldRestatingTheNamespaceRef(t *testing.T) {
	blob, err := json.Marshal(Arrow{
		Namespace:           Namespace("github.com/user/repo@v1.0.0"),
		InstalledConstraint: "v1.*",
	})
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(blob, &decoded))

	assert.NotContains(t, decoded, "resolved_branch")
	assert.NotContains(t, decoded, "installed_ref")
	assert.Equal(t, "github.com/user/repo@v1.0.0", decoded["namespace"])
	assert.Equal(t, "v1.*", decoded["installed_constraint"])

	for _, field := range []string{"ResolvedBranch", "InstalledRef"} {
		_, found := reflect.TypeOf(Arrow{}).FieldByName(field)
		assert.False(t, found, "Arrow must not declare a %s field", field)
	}
}

// Events written before those fields were removed still carry their keys. Asynx
// decodes aggregates with encoding/json, which ignores unknown keys, so replay
// must survive the old shape without an upcaster.
func TestArrow_UnmarshalsLegacyEventWithRemovedRefFields(t *testing.T) {
	legacy := []byte(`{
		"namespace": "github.com/user/repo@v1.0.0",
		"installed_at": "2026-04-11T15:33:00Z",
		"installed_ref": "v1.0.0",
		"resolved_branch": "master"
	}`)

	var arrow Arrow
	require.NoError(t, json.Unmarshal(legacy, &arrow))

	assert.Equal(t, Namespace("github.com/user/repo@v1.0.0"), arrow.Namespace)
	assert.Equal(t, "v1.0.0", arrow.Namespace.Ref())
	assert.False(t, arrow.InstalledAt.IsZero())
}

// The version-check fields round-trip through the same encoding/json path the
// Manifest JSON blob column relies on — a regression here breaks storage, not
// just serialization.
func TestArrow_JSONRoundTrip_VersionCheckFields(t *testing.T) {
	a := Arrow{
		Namespace:      Namespace("github.com/user/repo@main"),
		RefIsBranch:    true,
		RefCommitSHA:   "abc123",
		Outdated:       true,
		RecommendedRef: "v2.0.0",
	}

	data, err := json.Marshal(a)
	require.NoError(t, err)

	var got Arrow
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, a.RefIsBranch, got.RefIsBranch)
	assert.Equal(t, a.RefCommitSHA, got.RefCommitSHA)
	assert.Equal(t, a.Outdated, got.Outdated)
	assert.Equal(t, a.RecommendedRef, got.RecommendedRef)
}

func TestArrow_VersionCheckFields_ZeroValueByDefault(t *testing.T) {
	var a Arrow

	assert.False(t, a.RefIsBranch)
	assert.Empty(t, a.RefCommitSHA)
	assert.False(t, a.Outdated)
	assert.Empty(t, a.RecommendedRef)
}

// TestArrowState_TransitionsMatchSpecDiagram enforces the invariant
// docs/spec/domain.md states about itself: the mermaid state diagram there and
// the transitions map here are the same set of edges. The diagram had already
// gone stale twice — once when detached→running was added, and again for
// outdated→running — each time silently, because a diagram nothing reads back
// cannot fail. This makes it fail.
func TestArrowState_TransitionsMatchSpecDiagram(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "spec", "domain.md"))
	require.NoError(t, err)

	fromDiagram := parseArrowStateDiagram(t, string(raw))

	fromCode := map[string]bool{}
	for from, targets := range transitions {
		for _, to := range targets {
			fromCode[string(from)+"->"+string(to)] = true
		}
	}

	for edge := range fromCode {
		assert.True(t, fromDiagram[edge],
			"edge %q is in the transitions map but missing from docs/spec/domain.md", edge)
	}
	for edge := range fromDiagram {
		assert.True(t, fromCode[edge],
			"edge %q is drawn in docs/spec/domain.md but is not in the transitions map", edge)
	}
}

// parseArrowStateDiagram pulls the `from --> to` edges out of the arrow state
// diagram, ignoring the `[*]` start/terminal pseudo-states, which are notation
// rather than transitions.
func parseArrowStateDiagram(t *testing.T, doc string) map[string]bool {
	t.Helper()

	const open = "```mermaid\nstateDiagram-v2\n"
	start := strings.Index(doc, open)
	require.GreaterOrEqual(t, start, 0, "docs/spec/domain.md must contain the arrow state diagram")
	body := doc[start+len(open):]
	end := strings.Index(body, "```")
	require.GreaterOrEqual(t, end, 0, "the state diagram fence must be closed")

	edges := map[string]bool{}
	for _, line := range strings.Split(body[:end], "\n") {
		line = strings.TrimSpace(line)
		if i := strings.Index(line, " : "); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		from, to, ok := strings.Cut(line, " --> ")
		if !ok {
			continue
		}
		from, to = strings.TrimSpace(from), strings.TrimSpace(to)
		if from == "[*]" || to == "[*]" {
			continue
		}
		edges[from+"->"+to] = true
	}
	require.NotEmpty(t, edges, "the state diagram must declare at least one edge")
	return edges
}
