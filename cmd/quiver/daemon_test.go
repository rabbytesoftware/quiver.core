package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
