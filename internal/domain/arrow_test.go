package domain

import (
	"encoding/json"
	"reflect"
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
		{ArrowStateInstalling, ArrowStateReady},
		{ArrowStateInstalling, ArrowStateAbsent},
		{ArrowStateUninstalling, ArrowStateAbsent},
		{ArrowStateUninstalling, ArrowStateReady},
		{ArrowStateUpdating, ArrowStateReady},
		{ArrowStateUpdating, ArrowStateAbsent},
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
		{ArrowStateDetached, ArrowStateRunning},
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

// The ref a manifest was resolved at is the namespace's own ref on every path,
// so the aggregate must not carry a second field restating it — a duplicate is
// a place for the two answers to drift apart.
func TestArrow_HasNoResolvedBranchField(t *testing.T) {
	blob, err := json.Marshal(Arrow{
		Namespace:    Namespace("github.com/user/repo@v1.0.0"),
		InstalledRef: "v1.0.0",
	})
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(blob, &decoded))

	assert.NotContains(t, decoded, "resolved_branch")
	assert.Equal(t, "v1.0.0", decoded["installed_ref"])

	_, found := reflect.TypeOf(Arrow{}).FieldByName("ResolvedBranch")
	assert.False(t, found, "Arrow must not declare a ResolvedBranch field")
}

// An event written before the field was removed still carries the key. Asynx
// decodes aggregates with encoding/json, which ignores unknown keys, so replay
// must survive the old shape without an upcaster.
func TestArrow_UnmarshalsLegacyEventWithResolvedBranch(t *testing.T) {
	legacy := []byte(`{
		"namespace": "github.com/user/repo@v1.0.0",
		"installed_ref": "v1.0.0",
		"resolved_branch": "master"
	}`)

	var arrow Arrow
	require.NoError(t, json.Unmarshal(legacy, &arrow))

	assert.Equal(t, Namespace("github.com/user/repo@v1.0.0"), arrow.Namespace)
	assert.Equal(t, "v1.0.0", arrow.InstalledRef)
}
