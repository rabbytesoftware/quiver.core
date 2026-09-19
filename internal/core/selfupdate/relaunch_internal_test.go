package selfupdate

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Relaunch is the only caller of the per-OS handover, and the OS-independent
// half of it has to be provable without replacing the test process.
func TestRelaunch_DelegatesToTheHandover(t *testing.T) {
	var got string
	trig := NewTrigger(nil)
	trig.relaunch = func(binPath string) error {
		got = binPath
		return nil
	}

	trig.Fire("/vault/quiver-core/v2/quiver-new")

	require.NoError(t, trig.Relaunch())
	assert.Equal(t, "/vault/quiver-core/v2/quiver-new", got)
}

func TestRelaunch_PropagatesTheHandoverFailure(t *testing.T) {
	boom := errors.New("exec refused")
	trig := NewTrigger(nil)
	trig.relaunch = func(string) error { return boom }
	trig.Fire("/vault/quiver-core/v2/quiver-new")

	assert.ErrorIs(t, trig.Relaunch(), boom)
}

// A trigger that never fired must not reach the OS at all.
func TestRelaunch_NeverFired_SkipsTheHandover(t *testing.T) {
	called := false
	trig := NewTrigger(nil)
	trig.relaunch = func(string) error {
		called = true
		return nil
	}

	require.Error(t, trig.Relaunch())
	assert.False(t, called)
}

func TestNewTrigger_DefaultsToThePerOSHandover(t *testing.T) {
	assert.NotNil(t, NewTrigger(nil).relaunch)
}
