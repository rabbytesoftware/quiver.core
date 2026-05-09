package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
