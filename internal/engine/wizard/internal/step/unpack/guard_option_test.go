package unpack_test

import (
	"archive/tar"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack/unpacktest"
)

func extractTarWithGuard(
	t *testing.T,
	to string,
	entries []unpacktest.TarEntry,
	opts ...unpack.GuardOption,
) (*unpack.Guard, error) {
	t.Helper()

	from := unpacktest.WriteArchive(t, t.TempDir(), "a.tar", unpacktest.TarBytes(t, entries...))
	src, err := os.Open(from)
	require.NoError(t, err)
	defer src.Close()
	info, err := src.Stat()
	require.NoError(t, err)
	archive, err := unpack.DetectArchive(src, info.Size())
	require.NoError(t, err)

	g, err := unpack.OpenGuard(context.Background(), to, unpacktest.TestMaxBytes, opts...)
	require.NoError(t, err)
	t.Cleanup(g.Close)

	return g, archive.Extract(context.Background(), src, info.Size(), g)
}

func TestSkipEscapingLinks_SymlinkSkipsEscapingTargets(t *testing.T) {
	to := filepath.Join(t.TempDir(), "out")

	_, err := extractTarWithGuard(t, to, []unpacktest.TarEntry{
		{Name: "abs", Mode: 0o777, Flag: tar.TypeSymlink, Link: "/home/runner/x.png"},
		{Name: "up", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../../etc"},
		{Name: "d/self", Mode: 0o777, Flag: tar.TypeSymlink, Link: ".."},
		{Name: "d/self/esc", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../outside"},
		{Name: "file", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
		{Name: "inside", Mode: 0o777, Flag: tar.TypeSymlink, Link: "file"},
	}, unpack.SkipEscapingLinks())

	require.NoError(t, err)
	for _, name := range []string{"abs", "up", "esc"} {
		_, statErr := os.Lstat(filepath.Join(to, name))
		assert.ErrorIs(t, statErr, os.ErrNotExist, name)
	}
	target, err := os.Readlink(filepath.Join(to, "inside"))
	require.NoError(t, err)
	assert.Equal(t, "file", target)
}

func TestSkipEscapingLinks_SymlinkStillRejectsEmptyTarget(t *testing.T) {
	to := filepath.Join(t.TempDir(), "out")

	_, err := extractTarWithGuard(t, to, []unpacktest.TarEntry{
		{Name: "empty", Mode: 0o777, Flag: tar.TypeSymlink, Link: ""},
	}, unpack.SkipEscapingLinks())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid link target")
}

func TestSkipEscapingLinks_VerifyRemovesEscapingLinksSilently(t *testing.T) {
	dir := t.TempDir()
	to := filepath.Join(dir, "deep", "out")

	g, err := extractTarWithGuard(t, to, []unpacktest.TarEntry{
		{Name: "a/b/c/", Mode: 0o755, Flag: tar.TypeDir},
		{Name: "a/b/c/l", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../../.."},
		{Name: "e1", Mode: 0o777, Flag: tar.TypeSymlink, Link: "a/b/c/l/../../../../outside"},
		{Name: "kept", Mode: 0o777, Flag: tar.TypeSymlink, Link: "a/b"},
	}, unpack.SkipEscapingLinks())
	require.NoError(t, err)

	require.NoError(t, g.Verify())

	_, statErr := os.Lstat(filepath.Join(to, "e1"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
	_, statErr = os.Lstat(filepath.Join(to, "kept"))
	assert.NoError(t, statErr)
}

func TestSkipEscapingLinks_WithoutOptionVerifyStillFails(t *testing.T) {
	dir := t.TempDir()
	to := filepath.Join(dir, "deep", "out")

	g, err := extractTarWithGuard(t, to, []unpacktest.TarEntry{
		{Name: "a/b/c/", Mode: 0o755, Flag: tar.TypeDir},
		{Name: "a/b/c/l", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../../.."},
		{Name: "e1", Mode: 0o777, Flag: tar.TypeSymlink, Link: "a/b/c/l/../../../../outside"},
	})
	require.NoError(t, err)

	require.ErrorIs(t, g.Verify(), unpack.ErrEscape)
}

func TestSkipEscapingLinks_SkippedLinkLeavesTreeUntouched(t *testing.T) {
	to := filepath.Join(t.TempDir(), "out")

	_, err := extractTarWithGuard(t, to, []unpacktest.TarEntry{
		{Name: "esc", Body: "keep", Mode: 0o644, Flag: tar.TypeReg},
		{Name: "d/self", Mode: 0o777, Flag: tar.TypeSymlink, Link: ".."},
		{Name: "d/self/esc", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../outside"},
		{Name: "d/self/new/deeper/x", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../../../../outside"},
	}, unpack.SkipEscapingLinks())

	require.NoError(t, err)
	assert.Equal(t, "keep", unpacktest.ReadString(t, filepath.Join(to, "esc")))
	_, statErr := os.Lstat(filepath.Join(to, "new"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestGuard_Symlink_NewParentsResolvedBeforePlacing(t *testing.T) {
	to := filepath.Join(t.TempDir(), "out")

	_, err := extractTarWithGuard(t, to, []unpacktest.TarEntry{
		{Name: "a/b/c/l", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../../../file"},
	})

	require.NoError(t, err)
	target, err := os.Readlink(filepath.Join(to, "a", "b", "c", "l"))
	require.NoError(t, err)
	assert.Equal(t, "../../../file", target)
}

func TestGuard_Symlink_UnderRegularFileFails(t *testing.T) {
	to := filepath.Join(t.TempDir(), "out")

	_, err := extractTarWithGuard(t, to, []unpacktest.TarEntry{
		{Name: "f", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
		{Name: "f/l", Mode: 0o777, Flag: tar.TypeSymlink, Link: "x"},
	}, unpack.SkipEscapingLinks())

	require.Error(t, err)
	assert.NotErrorIs(t, err, unpack.ErrEscape)
	assert.Equal(t, "x", unpacktest.ReadString(t, filepath.Join(to, "f")))
}
