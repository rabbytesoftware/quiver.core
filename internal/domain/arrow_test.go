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

func TestArrowState_IsActive_StoppingReturnsTrue(t *testing.T) {
	assert.True(t, ArrowStateStopping.IsActive())
}

func TestArrowState_IsActive_ExecutingReturnsTrue(t *testing.T) {
	assert.True(t, ArrowStateExecuting.IsActive())
}

func TestArrowState_IsActive_ReadyReturnsFalse(t *testing.T) {
	assert.False(t, ArrowStateReady.IsActive())
}

func TestArrowState_IsActive_AbsentReturnsFalse(t *testing.T) {
	assert.False(t, ArrowStateAbsent.IsActive())
}

