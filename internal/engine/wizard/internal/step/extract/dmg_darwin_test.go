package extract_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	stepextract "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/extract"
)

func buildDmg(
	t *testing.T,
	src string,
	out string,
) {
	t.Helper()

	if _, err := exec.LookPath("hdiutil"); err != nil {
		t.Skip("hdiutil not available")
	}

	cmd := exec.Command("hdiutil", "create", "-quiet", "-srcfolder", src, "-format", "UDZO", out)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
}

func TestHandler_Execute_DmgCopiesBundle(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	versions := filepath.Join(src, "Foo.app", "Contents", "Frameworks", "X.framework", "Versions")
	require.NoError(t, os.MkdirAll(filepath.Join(versions, "A"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(versions, "A", "X"), []byte("lib"), 0o644))
	require.NoError(t, os.Symlink("A", filepath.Join(versions, "Current")))
	require.NoError(t, os.MkdirAll(filepath.Join(src, "Foo.app", "Contents", "MacOS"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "Foo.app", "Contents", "MacOS", "foo"), []byte("bin"), 0o755))
	require.NoError(t, os.Symlink("/Applications", filepath.Join(src, "Applications")))
	require.NoError(t, os.WriteFile(filepath.Join(src, ".hidden"), []byte("x"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(src, ".hiddendir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, ".hiddendir", "x"), []byte("x"), 0o644))
	image := filepath.Join(dir, "Foo.dmg")
	buildDmg(t, src, image)
	to := filepath.Join(dir, "out")

	require.NoError(t, runExtract(t, testMaxBytes, image, to, "5m"))

	bin, err := os.Stat(filepath.Join(to, "Foo.app", "Contents", "MacOS", "foo"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), bin.Mode().Perm())
	outVersions := filepath.Join(to, "Foo.app", "Contents", "Frameworks", "X.framework", "Versions")
	target, err := os.Readlink(filepath.Join(outVersions, "Current"))
	require.NoError(t, err)
	assert.Equal(t, "A", target)
	assert.Equal(t, "lib", readString(t, filepath.Join(outVersions, "Current", "X")))
	_, err = os.Lstat(filepath.Join(to, "Applications"))
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Lstat(filepath.Join(to, ".hidden"))
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Lstat(filepath.Join(to, ".hiddendir"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestHandler_Execute_DmgRejectsEscapingSymlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	require.NoError(t, os.MkdirAll(filepath.Join(src, "Foo.app"), 0o755))
	require.NoError(t, os.Symlink("../../../etc", filepath.Join(src, "Foo.app", "evil")))
	image := filepath.Join(dir, "Evil.dmg")
	buildDmg(t, src, image)

	err := runExtract(t, testMaxBytes, image, filepath.Join(dir, "out"), "5m")

	require.ErrorIs(t, err, stepextract.ErrEscape)
}

func TestHandler_Execute_DmgAttachFailure(t *testing.T) {
	dir := t.TempDir()
	image := writeArchive(t, dir, "broken.dmg", []byte("not a disk image"))

	err := runExtract(t, testMaxBytes, image, filepath.Join(dir, "out"), "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "extract: dmg: attach")
}
