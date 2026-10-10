//go:build darwin || linux

package launch

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func script(
	t *testing.T,
	dir string,
	name string,
	body string,
) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700)) // #nosec G306 -- test script must be executable
	return path
}

func TestStart_RunsTheTargetFromItsOwnDirectory(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "cwd.txt")
	target := script(t, dir, "app", `pwd > "`+out+`"`)

	require.NoError(t, Start(target))

	assert.Eventually(t, func() bool {
		data, err := os.ReadFile(out) // #nosec G304 -- file the test just asked the script to write
		return err == nil && len(data) > 0
	}, 5*time.Second, 20*time.Millisecond)
	data, err := os.ReadFile(out) // #nosec G304 -- test file
	require.NoError(t, err)
	want, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	got, err := filepath.EvalSymlinks(string(data[:len(data)-1]))
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestStart_ReportsASpawnFailure(t *testing.T) {
	err := Start(filepath.Join(t.TempDir(), "missing"))

	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrOpenFailed)
}

func TestStart_BundleGoesThroughOpen(t *testing.T) {
	dir := t.TempDir()
	args := filepath.Join(dir, "args.txt")
	fake := script(t, dir, "fake-open", `echo "$@" > "`+args+`"`)
	bundle := filepath.Join(dir, "Tool.app")
	swapBundleHost(t, fake, 5*time.Second)

	require.NoError(t, Start(bundle))

	data, err := os.ReadFile(args) // #nosec G304 -- file the fake open wrote
	require.NoError(t, err)
	assert.Equal(t, bundle+"\n", string(data))
}

func TestStart_BundleReportsOpenRefusing(t *testing.T) {
	dir := t.TempDir()
	fake := script(t, dir, "fake-open", "exit 1")
	swapBundleHost(t, fake, 5*time.Second)

	err := Start(filepath.Join(dir, "Tool.app"))

	assert.ErrorIs(t, err, ErrOpenFailed)
}

func TestStart_BundleStillRunningAfterTheWaitIsSuccess(t *testing.T) {
	dir := t.TempDir()
	fake := script(t, dir, "fake-open", "sleep 5")
	swapBundleHost(t, fake, 100*time.Millisecond)

	assert.NoError(t, Start(filepath.Join(dir, "Tool.app")))
}

func TestIsBundle(t *testing.T) {
	swapBundleHost(t, "unused", time.Second)

	assert.True(t, IsBundle("/Applications/Tool.app"))
	assert.True(t, IsBundle("/Applications/Tool.APP"))
	assert.False(t, IsBundle("/Applications/Tool"))

	bundleHost = false
	assert.False(t, IsBundle("/Applications/Tool.app"), "only macOS opens bundles")
}

func swapBundleHost(
	t *testing.T,
	open string,
	wait time.Duration,
) {
	t.Helper()
	prevOpen, prevHost, prevWait := openCommand, bundleHost, openWait
	openCommand, bundleHost, openWait = open, true, wait
	t.Cleanup(func() { openCommand, bundleHost, openWait = prevOpen, prevHost, prevWait })
}
