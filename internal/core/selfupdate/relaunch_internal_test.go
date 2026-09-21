package selfupdate

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Relaunch is the only caller of the per-OS handover, and the OS-independent
// half of it has to be provable without replacing the test process.
func TestRelaunch_DelegatesToTheHandover(t *testing.T) {
	var got string
	trig := NewTrigger(nil)
	trig.handOver = func(binPath string) error {
		got = binPath
		return nil
	}

	trig.Fire("/vault/quiver-core/v2/quiver-new")

	require.NoError(t, trig.Relaunch())
	assert.Equal(t, "/vault/quiver-core/v2/quiver-new", got)
}

// A trigger that never fired must not reach the OS at all.
func TestRelaunch_NeverFired_SkipsTheHandover(t *testing.T) {
	called := false
	trig := NewTrigger(nil)
	trig.handOver = func(string) error {
		called = true
		return nil
	}

	require.Error(t, trig.Relaunch())
	assert.False(t, called)
}

func TestRelaunch_HandOverSucceeds_NeverFallsBack(t *testing.T) {
	resumed := false
	trig := NewTrigger(nil)
	trig.handOver = func(string) error { return nil }
	trig.resume = func(string) error {
		resumed = true
		return nil
	}
	trig.Fire("/vault/quiver-core/v2/quiver-new")

	require.NoError(t, trig.Relaunch())
	assert.False(t, resumed, "a handover that worked has already replaced this process")
}

// The daemon has no supervisor behind it. A handover that fails — a truncated
// download, a read-only vault, a swept workdir — must put the old build back
// rather than leave the machine with no quiver at all.
func TestRelaunch_HandOverFails_ResumesTheOldBinary(t *testing.T) {
	handOverErr := errors.New("exec format error")

	var resumed string
	trig := NewTrigger(nil)
	trig.handOver = func(string) error { return handOverErr }
	trig.resume = func(binPath string) error {
		resumed = binPath
		return nil
	}
	trig.Fire("/vault/quiver-core/v2/quiver-new")

	err := trig.Relaunch()

	current, exeErr := os.Executable()
	require.NoError(t, exeErr)
	assert.Equal(t, current, resumed, "the fallback must exec the binary still running, not the new one")
	assert.ErrorIs(t, err, handOverErr, "a failed update must still be surfaced, not swallowed by a working fallback")
}

func TestRelaunch_HandOverFailsAndResumeFails_ReportsBoth(t *testing.T) {
	handOverErr := errors.New("exec format error")
	resumeErr := errors.New("old binary gone too")

	trig := NewTrigger(nil)
	trig.handOver = func(string) error { return handOverErr }
	trig.resume = func(string) error { return resumeErr }
	trig.Fire("/vault/quiver-core/v2/quiver-new")

	err := trig.Relaunch()

	assert.ErrorIs(t, err, handOverErr)
	assert.ErrorIs(t, err, resumeErr)
}

func TestRelaunch_HandOverFailsAndCurrentBinaryUnknown_ReportsBoth(t *testing.T) {
	handOverErr := errors.New("exec format error")
	lookupErr := errors.New("no executable path")

	resumed := false
	trig := NewTrigger(nil)
	trig.handOver = func(string) error { return handOverErr }
	trig.currentBinary = func() (string, error) { return "", lookupErr }
	trig.resume = func(string) error {
		resumed = true
		return nil
	}
	trig.Fire("/vault/quiver-core/v2/quiver-new")

	err := trig.Relaunch()

	assert.ErrorIs(t, err, handOverErr)
	assert.ErrorIs(t, err, lookupErr)
	assert.False(t, resumed, "there is nothing to resume when the current binary cannot be located")
}

func TestNewTrigger_DefaultsToThePerOSHandover(t *testing.T) {
	trig := NewTrigger(nil)

	assert.NotNil(t, trig.handOver)
	assert.NotNil(t, trig.resume)
	assert.NotNil(t, trig.currentBinary)
}
