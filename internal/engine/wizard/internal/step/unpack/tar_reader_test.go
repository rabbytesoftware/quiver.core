package unpack_test

import (
	"archive/tar"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack/unpacktest"
)

func TestArchive_Extract_TarPreservesLayoutAndModes(t *testing.T) {
	dir := t.TempDir()
	from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.TarBytes(t,
		unpacktest.TarEntry{Name: "./", Mode: 0o755, Flag: tar.TypeDir},
		unpacktest.TarEntry{Name: "pkg/", Mode: 0o755, Flag: tar.TypeDir},
		unpacktest.TarEntry{Name: "pkg/bin/tool", Body: "#!/bin/sh\n", Mode: 0o755, Flag: tar.TypeReg},
		unpacktest.TarEntry{Name: "pkg/README", Body: "docs", Mode: 0o644, Flag: tar.TypeReg},
		unpacktest.TarEntry{Name: "pkg/current", Mode: 0o777, Flag: tar.TypeSymlink, Link: "bin/tool"},
		unpacktest.TarEntry{Name: "pkg/tool-hard", Mode: 0o755, Flag: tar.TypeLink, Link: "pkg/bin/tool"},
		unpacktest.TarEntry{Name: "pkg/fifo", Mode: 0o644, Flag: tar.TypeFifo},
	))
	to := filepath.Join(dir, "out")

	require.NoError(t, runUnpack(t, unpacktest.TestMaxBytes, from, to, 0))

	tool, err := os.Stat(filepath.Join(to, "pkg", "bin", "tool"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), tool.Mode().Perm())

	readme, err := os.Stat(filepath.Join(to, "pkg", "README"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), readme.Mode().Perm())

	target, err := os.Readlink(filepath.Join(to, "pkg", "current"))
	require.NoError(t, err)
	assert.Equal(t, "bin/tool", target)

	assert.Equal(t, "#!/bin/sh\n", unpacktest.ReadString(t, filepath.Join(to, "pkg", "tool-hard")))

	_, err = os.Lstat(filepath.Join(to, "pkg", "fifo"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestArchive_Extract_TarOverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	to := filepath.Join(dir, "out")
	require.NoError(t, os.MkdirAll(to, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(to, "hello.txt"), []byte("old"), 0o600))
	from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.HelloTar(t))

	require.NoError(t, runUnpack(t, unpacktest.TestMaxBytes, from, to, 0))

	assert.Equal(t, "hello\n", unpacktest.ReadString(t, filepath.Join(to, "hello.txt")))
}

func TestArchive_Extract_TarRejectsEscapes(t *testing.T) {
	testCases := []struct {
		name  string
		entry unpacktest.TarEntry
	}{
		{name: "parent traversal", entry: unpacktest.TarEntry{Name: "../x", Body: "pwn", Mode: 0o644, Flag: tar.TypeReg}},
		{
			name:  "nested traversal",
			entry: unpacktest.TarEntry{Name: "a/../../x", Body: "pwn", Mode: 0o644, Flag: tar.TypeReg},
		},
		{name: "absolute path", entry: unpacktest.TarEntry{Name: "/x", Body: "pwn", Mode: 0o644, Flag: tar.TypeReg}},
		{name: "traversal dir", entry: unpacktest.TarEntry{Name: "../x/", Mode: 0o755, Flag: tar.TypeDir}},
		{
			name:  "absolute symlink",
			entry: unpacktest.TarEntry{Name: "x", Mode: 0o777, Flag: tar.TypeSymlink, Link: "/etc"},
		},
		{
			name:  "relative symlink",
			entry: unpacktest.TarEntry{Name: "a/x", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../../x"},
		},
		{
			name:  "symlink named outside",
			entry: unpacktest.TarEntry{Name: "../x", Mode: 0o777, Flag: tar.TypeSymlink, Link: "y"},
		},
		{
			name:  "relative hardlink",
			entry: unpacktest.TarEntry{Name: "x", Mode: 0o644, Flag: tar.TypeLink, Link: "../x"},
		},
		{
			name:  "absolute hardlink",
			entry: unpacktest.TarEntry{Name: "x", Mode: 0o644, Flag: tar.TypeLink, Link: "/etc/passwd"},
		},
		{
			name:  "hardlink named outside",
			entry: unpacktest.TarEntry{Name: "../x", Mode: 0o644, Flag: tar.TypeLink, Link: "y"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.TarBytes(t, tc.entry))
			to := filepath.Join(dir, "deep", "out")

			err := runUnpack(t, unpacktest.TestMaxBytes, from, to, 0)

			require.ErrorIs(t, err, unpack.ErrEscape)
			_, statErr := os.Lstat(filepath.Join(dir, "deep", "x"))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
			_, statErr = os.Lstat(filepath.Join(dir, "x"))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func TestArchive_Extract_TarRejectsInvalidEntries(t *testing.T) {
	testCases := []struct {
		name    string
		entries []unpacktest.TarEntry
		wantMsg string
	}{
		{
			name:    "file named dot",
			entries: []unpacktest.TarEntry{{Name: ".", Body: "x", Mode: 0o644, Flag: tar.TypeReg}},
			wantMsg: "invalid entry name",
		},
		{
			name:    "empty symlink target",
			entries: []unpacktest.TarEntry{{Name: "x", Mode: 0o777, Flag: tar.TypeSymlink, Link: ""}},
			wantMsg: "invalid link target",
		},
		{
			name:    "hardlink to root",
			entries: []unpacktest.TarEntry{{Name: "x", Mode: 0o644, Flag: tar.TypeLink, Link: "./"}},
			wantMsg: "invalid link target",
		},
		{
			name:    "hardlink to missing entry",
			entries: []unpacktest.TarEntry{{Name: "x", Mode: 0o644, Flag: tar.TypeLink, Link: "missing"}},
			wantMsg: "unpack: link",
		},
		{
			name: "hardlink to symlink",
			entries: []unpacktest.TarEntry{
				{Name: "s", Mode: 0o777, Flag: tar.TypeSymlink, Link: "hello.txt"},
				{Name: "x", Mode: 0o644, Flag: tar.TypeLink, Link: "s"},
			},
			wantMsg: "hardlink to symlink",
		},
		{
			name: "file replacing non-empty directory",
			entries: []unpacktest.TarEntry{
				{Name: "d/child", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
				{Name: "d", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
			},
			wantMsg: "unpack: create",
		},
		{
			name: "directory replacing file",
			entries: []unpacktest.TarEntry{
				{Name: "d", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
				{Name: "d/", Mode: 0o755, Flag: tar.TypeDir},
			},
			wantMsg: "unpack: mkdir",
		},
		{
			name: "symlink replacing non-empty directory",
			entries: []unpacktest.TarEntry{
				{Name: "d/child", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
				{Name: "d", Mode: 0o777, Flag: tar.TypeSymlink, Link: "child"},
			},
			wantMsg: "unpack: symlink",
		},
		{
			name: "symlink under a file",
			entries: []unpacktest.TarEntry{
				{Name: "d", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
				{Name: "d/l", Mode: 0o777, Flag: tar.TypeSymlink, Link: "x"},
			},
			wantMsg: "unpack: mkdir",
		},
		{
			name: "hardlink under a file",
			entries: []unpacktest.TarEntry{
				{Name: "d", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
				{Name: "d/l", Mode: 0o644, Flag: tar.TypeLink, Link: "d"},
			},
			wantMsg: "unpack: mkdir",
		},
		{
			name: "hardlink to directory",
			entries: []unpacktest.TarEntry{
				{Name: "sub/", Mode: 0o755, Flag: tar.TypeDir},
				{Name: "x", Mode: 0o644, Flag: tar.TypeLink, Link: "sub"},
			},
			wantMsg: "unpack: link x",
		},
		{
			name: "file under a file",
			entries: []unpacktest.TarEntry{
				{Name: "d", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
				{Name: "d/child", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
			},
			wantMsg: "unpack: mkdir",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.TarBytes(t, tc.entries...))

			err := runUnpack(t, unpacktest.TestMaxBytes, from, filepath.Join(dir, "out"), 0)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

func TestArchive_Extract_TarTruncated(t *testing.T) {
	dir := t.TempDir()
	from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.HelloTar(t)[:515])

	err := runUnpack(t, unpacktest.TestMaxBytes, from, filepath.Join(dir, "out"), 0)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unpack")
}

func TestArchive_Extract_TarSkippedBodiesCountTowardsCeiling(t *testing.T) {
	testCases := []struct {
		name     string
		maxBytes int64
		wantErr  error
	}{
		{name: "within ceiling", maxBytes: unpacktest.TestMaxBytes},
		{name: "over ceiling", maxBytes: 1024, wantErr: unpack.ErrTooLarge},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.TarBytes(t,
				unpacktest.TarEntry{Name: "opaque", Body: strings.Repeat("0", 4096), Mode: 0o644, Flag: 'Z'},
				unpacktest.TarEntry{Name: "hello.txt", Body: "hello\n", Mode: 0o644, Flag: tar.TypeReg},
			))
			to := filepath.Join(dir, "out")

			err := runUnpack(t, tc.maxBytes, from, to, 0)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "hello\n", unpacktest.ReadString(t, filepath.Join(to, "hello.txt")))
			_, statErr := os.Lstat(filepath.Join(to, "opaque"))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}
