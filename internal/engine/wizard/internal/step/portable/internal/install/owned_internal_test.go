package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestFile(
	t *testing.T,
	path string,
	body string,
) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
}

func readTestFile(
	t *testing.T,
	path string,
) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(data)
}

func TestSwapOwned_FailedMoveIntoPlaceRestoresThePreviousInstall(t *testing.T) {
	dir := t.TempDir()
	to := filepath.Join(dir, "tool")
	aside := sibling(to, asideSuffix)
	writeTestFile(t, filepath.Join(to, "tool"), "v1")

	err := swapOwned(filepath.Join(dir, "missing-staging"), to, aside)

	require.ErrorIs(t, err, os.ErrNotExist)
	assert.Equal(t, "v1", readTestFile(t, filepath.Join(to, "tool")))
	assert.NoDirExists(t, aside)
}

func TestSwapOwned_PreviousInstallThatCannotMoveAsideStaysInPlace(t *testing.T) {
	dir := t.TempDir()
	to := filepath.Join(dir, "tool")
	aside := sibling(to, asideSuffix)
	staging := sibling(to, stagingSuffix)
	writeTestFile(t, filepath.Join(to, "tool"), "v1")
	writeTestFile(t, filepath.Join(aside, "busy"), "x")
	writeTestFile(t, filepath.Join(staging, "tool"), "v2")

	err := swapOwned(staging, to, aside)

	require.Error(t, err)
	assert.Equal(t, "v1", readTestFile(t, filepath.Join(to, "tool")))
	assert.Equal(t, "v2", readTestFile(t, filepath.Join(staging, "tool")))
}

func TestSwapOwned_FreshDestination(t *testing.T) {
	dir := t.TempDir()
	to := filepath.Join(dir, "tool")
	staging := sibling(to, stagingSuffix)
	writeTestFile(t, filepath.Join(staging, "tool"), "v1")

	require.NoError(t, swapOwned(staging, to, sibling(to, asideSuffix)))

	assert.Equal(t, "v1", readTestFile(t, filepath.Join(to, "tool")))
	assert.NoDirExists(t, staging)
}
