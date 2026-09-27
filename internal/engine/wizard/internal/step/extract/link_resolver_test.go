package extract_test

import (
	"archive/tar"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	stepextract "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/extract"
)

func TestHandler_Execute_KeepsLinksResolvingInside(t *testing.T) {
	testCases := []struct {
		name    string
		entries []tarEntry
		check   string
	}{
		{
			name: "chain through directories and links",
			entries: []tarEntry{
				{name: "a/b/c/", mode: 0o755, flag: tar.TypeDir},
				{name: "a/b/c/l", mode: 0o777, flag: tar.TypeSymlink, link: "../../.."},
				{name: "hello.txt", body: "hello\n", mode: 0o644, flag: tar.TypeReg},
				{name: "y", mode: 0o777, flag: tar.TypeSymlink, link: "a/b/c/l/./a//b/../../hello.txt"},
			},
			check: "y",
		},
		{
			name: "dangling through a dangling link",
			entries: []tarEntry{
				{name: "l", mode: 0o777, flag: tar.TypeSymlink, link: "missing/deeper"},
				{name: "y", mode: 0o777, flag: tar.TypeSymlink, link: "l/../z"},
			},
			check: "y",
		},
		{
			name: "link replaced by a file",
			entries: []tarEntry{
				{name: "x", mode: 0o777, flag: tar.TypeSymlink, link: "hello.txt"},
				{name: "x", body: "file", mode: 0o644, flag: tar.TypeReg},
			},
			check: "x",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := writeArchive(t, dir, "a.tar", tarBytes(t, tc.entries...))
			to := filepath.Join(dir, "out")

			require.NoError(t, runExtract(t, testMaxBytes, from, to, ""))

			_, err := os.Lstat(filepath.Join(to, tc.check))
			assert.NoError(t, err)
		})
	}
}

func TestHandler_Execute_RejectsLinkThroughPreexistingAbsoluteSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0o600))
	to := filepath.Join(dir, "out")
	require.NoError(t, os.MkdirAll(to, 0o755))
	require.NoError(t, os.Symlink(outside, filepath.Join(to, "ext")))
	from := writeArchive(t, dir, "a.tar", tarBytes(t,
		tarEntry{name: "x", mode: 0o777, flag: tar.TypeSymlink, link: "ext/secret"},
	))

	err := runExtract(t, testMaxBytes, from, to, "")

	require.ErrorIs(t, err, stepextract.ErrEscape)
	_, statErr := os.Lstat(filepath.Join(to, "x"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}
