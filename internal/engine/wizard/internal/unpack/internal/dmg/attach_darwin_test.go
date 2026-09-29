package dmg_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/dmg"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/guard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
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

func runExtractDmg(
	t *testing.T,
	image string,
	to string,
	timeout time.Duration,
) error {
	t.Helper()

	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	src, err := os.Open(image)
	require.NoError(t, err)
	defer src.Close()
	info, err := src.Stat()
	require.NoError(t, err)

	format, ok, err := dmg.New(mocks.TestMaxBytes, guard.NameRules{FoldCase: true})(src, info.Size())
	require.NoError(t, err)
	require.True(t, ok)

	_, err = format.Unpack(ctx, models.Target{Dir: to})

	return err
}

func TestExtractDmg_CopiesBundle(t *testing.T) {
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

	require.NoError(t, runExtractDmg(t, image, to, 5*time.Minute))

	bin, err := os.Stat(filepath.Join(to, "Foo.app", "Contents", "MacOS", "foo"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), bin.Mode().Perm())
	outVersions := filepath.Join(to, "Foo.app", "Contents", "Frameworks", "X.framework", "Versions")
	target, err := os.Readlink(filepath.Join(outVersions, "Current"))
	require.NoError(t, err)
	assert.Equal(t, "A", target)
	assert.Equal(t, "lib", mocks.ReadString(t, filepath.Join(outVersions, "Current", "X")))
	_, err = os.Lstat(filepath.Join(to, "Applications"))
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Lstat(filepath.Join(to, ".hidden"))
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Lstat(filepath.Join(to, ".hiddendir"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestExtractDmg_RejectsEscapingSymlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	require.NoError(t, os.MkdirAll(filepath.Join(src, "Foo.app"), 0o755))
	require.NoError(t, os.Symlink("../../../etc", filepath.Join(src, "Foo.app", "evil")))
	image := filepath.Join(dir, "Evil.dmg")
	buildDmg(t, src, image)

	err := runExtractDmg(t, image, filepath.Join(dir, "out"), 5*time.Minute)

	require.ErrorIs(t, err, models.ErrEscape)
}

func TestExtractDmg_AttachFailure(t *testing.T) {
	dir := t.TempDir()
	image := mocks.WriteFile(t, filepath.Join(dir, "broken.dmg"), append([]byte("not a disk image"), mocks.DmgTrailer()...))
	to := filepath.Join(dir, "out")

	err := runExtractDmg(t, image, to, 0)

	require.Error(t, err)
	entries, rdErr := os.ReadDir(to)
	require.NoError(t, rdErr)
	assert.Empty(t, entries)
}
