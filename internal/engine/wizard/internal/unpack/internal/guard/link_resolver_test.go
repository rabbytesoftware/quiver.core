package guard_test

import (
	"archive/tar"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/unpacktest"
)

func TestArchive_Extract_KeepsLinksResolvingInside(t *testing.T) {
	testCases := []struct {
		name    string
		entries []unpacktest.TarEntry
		check   string
	}{
		{
			name: "chain through directories and links",
			entries: []unpacktest.TarEntry{
				{Name: "a/b/c/", Mode: 0o755, Flag: tar.TypeDir},
				{Name: "a/b/c/l", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../../.."},
				{Name: "hello.txt", Body: "hello\n", Mode: 0o644, Flag: tar.TypeReg},
				{Name: "y", Mode: 0o777, Flag: tar.TypeSymlink, Link: "a/b/c/l/./a//b/../../hello.txt"},
			},
			check: "y",
		},
		{
			name: "dangling through a dangling link",
			entries: []unpacktest.TarEntry{
				{Name: "l", Mode: 0o777, Flag: tar.TypeSymlink, Link: "missing/deeper"},
				{Name: "y", Mode: 0o777, Flag: tar.TypeSymlink, Link: "l/../z"},
			},
			check: "y",
		},
		{
			name: "link replaced by a file",
			entries: []unpacktest.TarEntry{
				{Name: "x", Mode: 0o777, Flag: tar.TypeSymlink, Link: "hello.txt"},
				{Name: "x", Body: "file", Mode: 0o644, Flag: tar.TypeReg},
			},
			check: "x",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.TarBytes(t, tc.entries...))
			to := filepath.Join(dir, "out")

			require.NoError(t, runUnpack(t, unpacktest.TestMaxBytes, from, to, 0))

			_, err := os.Lstat(filepath.Join(to, tc.check))
			assert.NoError(t, err)
		})
	}
}

func TestArchive_Extract_RejectsLinkThroughPreexistingAbsoluteSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0o600))
	to := filepath.Join(dir, "out")
	require.NoError(t, os.MkdirAll(to, 0o755))
	require.NoError(t, os.Symlink(outside, filepath.Join(to, "ext")))
	from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.TarBytes(t,
		unpacktest.TarEntry{Name: "x", Mode: 0o777, Flag: tar.TypeSymlink, Link: "ext/secret"},
	))

	err := runUnpack(t, unpacktest.TestMaxBytes, from, to, 0)

	require.ErrorIs(t, err, models.ErrEscape)
	_, statErr := os.Lstat(filepath.Join(to, "x"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}
