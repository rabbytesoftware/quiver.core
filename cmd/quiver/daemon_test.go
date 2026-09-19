package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/core/selfupdate"
)

func TestScopeDevHome_DevVersion_SetsQuiverHome(t *testing.T) {
	os.Unsetenv("QUIVER_HOME")
	t.Cleanup(func() { os.Unsetenv("QUIVER_HOME") })
	t.Chdir(t.TempDir())

	cwd, err := os.Getwd()
	require.NoError(t, err)

	require.NoError(t, scopeDevHome("dev"))
	assert.Equal(t, filepath.Join(cwd, ".quiver"), os.Getenv("QUIVER_HOME"))
}

func TestScopeDevHome_RealVersion_LeavesQuiverHomeUnset(t *testing.T) {
	os.Unsetenv("QUIVER_HOME")
	t.Cleanup(func() { os.Unsetenv("QUIVER_HOME") })

	require.NoError(t, scopeDevHome("v1.2.3"))
	_, ok := os.LookupEnv("QUIVER_HOME")
	assert.False(t, ok)
}

func TestScopeDevHome_QuiverHomeAlreadySet_LeftAlone(t *testing.T) {
	t.Setenv("QUIVER_HOME", "/already/set")

	require.NoError(t, scopeDevHome("dev"))
	assert.Equal(t, "/already/set", os.Getenv("QUIVER_HOME"))
}

// A daemon that was not replaced must come out of Start exactly as it always
// did, carrying Start's own error and nothing else.
func TestSucceedIfUpdated_NeverFired_ReturnsStartError(t *testing.T) {
	startErr := errors.New("listener died")

	err := succeedIfUpdated(selfupdate.NewTrigger(nil), startErr)

	assert.Same(t, startErr, err)
}

func TestSucceedIfUpdated_NeverFired_NoStartError_ReturnsNil(t *testing.T) {
	assert.NoError(t, succeedIfUpdated(selfupdate.NewTrigger(nil), nil))
}

// A handover that cannot happen must surface, not be swallowed behind a clean
// Start: the operator's daemon is gone and its replacement never arrived.
func TestSucceedIfUpdated_RelaunchFails_ReturnsTheRelaunchError(t *testing.T) {
	trig := selfupdate.NewTrigger(nil)
	trig.Fire("")

	err := succeedIfUpdated(trig, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no binary path recorded")
}

func TestSucceedIfUpdated_RelaunchFails_KeepsTheStartError(t *testing.T) {
	startErr := errors.New("listener died")
	trig := selfupdate.NewTrigger(nil)
	trig.Fire("")

	err := succeedIfUpdated(trig, startErr)

	assert.ErrorIs(t, err, startErr)
}
