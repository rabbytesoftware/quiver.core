package extract_test

import (
	"archive/tar"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	stepextract "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/extract"
)

func TestHandler_Execute_TarPreservesLayoutAndModes(t *testing.T) {
	dir := t.TempDir()
	from := writeArchive(t, dir, "a.tar", tarBytes(t,
		tarEntry{name: "./", mode: 0o755, flag: tar.TypeDir},
		tarEntry{name: "pkg/", mode: 0o755, flag: tar.TypeDir},
		tarEntry{name: "pkg/bin/tool", body: "#!/bin/sh\n", mode: 0o755, flag: tar.TypeReg},
		tarEntry{name: "pkg/README", body: "docs", mode: 0o644, flag: tar.TypeReg},
		tarEntry{name: "pkg/current", mode: 0o777, flag: tar.TypeSymlink, link: "bin/tool"},
		tarEntry{name: "pkg/tool-hard", mode: 0o755, flag: tar.TypeLink, link: "pkg/bin/tool"},
		tarEntry{name: "pkg/fifo", mode: 0o644, flag: tar.TypeFifo},
	))
	to := filepath.Join(dir, "out")

	require.NoError(t, runExtract(t, testMaxBytes, from, to, ""))

	tool, err := os.Stat(filepath.Join(to, "pkg", "bin", "tool"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), tool.Mode().Perm())

	readme, err := os.Stat(filepath.Join(to, "pkg", "README"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), readme.Mode().Perm())

	target, err := os.Readlink(filepath.Join(to, "pkg", "current"))
	require.NoError(t, err)
	assert.Equal(t, "bin/tool", target)

	assert.Equal(t, "#!/bin/sh\n", readString(t, filepath.Join(to, "pkg", "tool-hard")))

	_, err = os.Lstat(filepath.Join(to, "pkg", "fifo"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestHandler_Execute_TarOverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	to := filepath.Join(dir, "out")
	require.NoError(t, os.MkdirAll(to, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(to, "hello.txt"), []byte("old"), 0o600))
	from := writeArchive(t, dir, "a.tar", helloTar(t))

	require.NoError(t, runExtract(t, testMaxBytes, from, to, ""))

	assert.Equal(t, "hello\n", readString(t, filepath.Join(to, "hello.txt")))
}

func TestHandler_Execute_TarRejectsEscapes(t *testing.T) {
	testCases := []struct {
		name  string
		entry tarEntry
	}{
		{name: "parent traversal", entry: tarEntry{name: "../x", body: "pwn", mode: 0o644, flag: tar.TypeReg}},
		{name: "nested traversal", entry: tarEntry{name: "a/../../x", body: "pwn", mode: 0o644, flag: tar.TypeReg}},
		{name: "absolute path", entry: tarEntry{name: "/x", body: "pwn", mode: 0o644, flag: tar.TypeReg}},
		{name: "traversal dir", entry: tarEntry{name: "../x/", mode: 0o755, flag: tar.TypeDir}},
		{name: "absolute symlink", entry: tarEntry{name: "x", mode: 0o777, flag: tar.TypeSymlink, link: "/etc"}},
		{name: "relative symlink", entry: tarEntry{name: "a/x", mode: 0o777, flag: tar.TypeSymlink, link: "../../x"}},
		{name: "symlink named outside", entry: tarEntry{name: "../x", mode: 0o777, flag: tar.TypeSymlink, link: "y"}},
		{name: "relative hardlink", entry: tarEntry{name: "x", mode: 0o644, flag: tar.TypeLink, link: "../x"}},
		{name: "absolute hardlink", entry: tarEntry{name: "x", mode: 0o644, flag: tar.TypeLink, link: "/etc/passwd"}},
		{name: "hardlink named outside", entry: tarEntry{name: "../x", mode: 0o644, flag: tar.TypeLink, link: "y"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := writeArchive(t, dir, "a.tar", tarBytes(t, tc.entry))
			to := filepath.Join(dir, "deep", "out")

			err := runExtract(t, testMaxBytes, from, to, "")

			require.ErrorIs(t, err, stepextract.ErrEscape)
			_, statErr := os.Lstat(filepath.Join(dir, "deep", "x"))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
			_, statErr = os.Lstat(filepath.Join(dir, "x"))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func TestHandler_Execute_TarRejectsInvalidEntries(t *testing.T) {
	testCases := []struct {
		name    string
		entries []tarEntry
		wantMsg string
	}{
		{
			name:    "file named dot",
			entries: []tarEntry{{name: ".", body: "x", mode: 0o644, flag: tar.TypeReg}},
			wantMsg: "invalid entry name",
		},
		{
			name:    "empty symlink target",
			entries: []tarEntry{{name: "x", mode: 0o777, flag: tar.TypeSymlink, link: ""}},
			wantMsg: "invalid link target",
		},
		{
			name:    "hardlink to root",
			entries: []tarEntry{{name: "x", mode: 0o644, flag: tar.TypeLink, link: "./"}},
			wantMsg: "invalid link target",
		},
		{
			name:    "hardlink to missing entry",
			entries: []tarEntry{{name: "x", mode: 0o644, flag: tar.TypeLink, link: "missing"}},
			wantMsg: "extract: link",
		},
		{
			name: "hardlink to symlink",
			entries: []tarEntry{
				{name: "s", mode: 0o777, flag: tar.TypeSymlink, link: "hello.txt"},
				{name: "x", mode: 0o644, flag: tar.TypeLink, link: "s"},
			},
			wantMsg: "hardlink to symlink",
		},
		{
			name: "file replacing non-empty directory",
			entries: []tarEntry{
				{name: "d/child", body: "x", mode: 0o644, flag: tar.TypeReg},
				{name: "d", body: "x", mode: 0o644, flag: tar.TypeReg},
			},
			wantMsg: "extract: create",
		},
		{
			name: "directory replacing file",
			entries: []tarEntry{
				{name: "d", body: "x", mode: 0o644, flag: tar.TypeReg},
				{name: "d/", mode: 0o755, flag: tar.TypeDir},
			},
			wantMsg: "extract: mkdir",
		},
		{
			name: "symlink replacing non-empty directory",
			entries: []tarEntry{
				{name: "d/child", body: "x", mode: 0o644, flag: tar.TypeReg},
				{name: "d", mode: 0o777, flag: tar.TypeSymlink, link: "child"},
			},
			wantMsg: "extract: symlink",
		},
		{
			name: "symlink under a file",
			entries: []tarEntry{
				{name: "d", body: "x", mode: 0o644, flag: tar.TypeReg},
				{name: "d/l", mode: 0o777, flag: tar.TypeSymlink, link: "x"},
			},
			wantMsg: "extract: mkdir",
		},
		{
			name: "hardlink under a file",
			entries: []tarEntry{
				{name: "d", body: "x", mode: 0o644, flag: tar.TypeReg},
				{name: "d/l", mode: 0o644, flag: tar.TypeLink, link: "d"},
			},
			wantMsg: "extract: mkdir",
		},
		{
			name: "hardlink to directory",
			entries: []tarEntry{
				{name: "sub/", mode: 0o755, flag: tar.TypeDir},
				{name: "x", mode: 0o644, flag: tar.TypeLink, link: "sub"},
			},
			wantMsg: "extract: link x",
		},
		{
			name: "file under a file",
			entries: []tarEntry{
				{name: "d", body: "x", mode: 0o644, flag: tar.TypeReg},
				{name: "d/child", body: "x", mode: 0o644, flag: tar.TypeReg},
			},
			wantMsg: "extract: mkdir",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := writeArchive(t, dir, "a.tar", tarBytes(t, tc.entries...))

			err := runExtract(t, testMaxBytes, from, filepath.Join(dir, "out"), "")

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

func TestHandler_Execute_TarTruncated(t *testing.T) {
	dir := t.TempDir()
	from := writeArchive(t, dir, "a.tar", helloTar(t)[:515])

	err := runExtract(t, testMaxBytes, from, filepath.Join(dir, "out"), "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "extract")
}

func TestHandler_Execute_TarSkippedBodiesCountTowardsCeiling(t *testing.T) {
	testCases := []struct {
		name     string
		maxBytes int64
		wantErr  error
	}{
		{name: "within ceiling", maxBytes: testMaxBytes},
		{name: "over ceiling", maxBytes: 1024, wantErr: stepextract.ErrTooLarge},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := writeArchive(t, dir, "a.tar", tarBytes(t,
				tarEntry{name: "opaque", body: strings.Repeat("0", 4096), mode: 0o644, flag: 'Z'},
				tarEntry{name: "hello.txt", body: "hello\n", mode: 0o644, flag: tar.TypeReg},
			))
			to := filepath.Join(dir, "out")

			err := runExtract(t, tc.maxBytes, from, to, "")

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "hello\n", readString(t, filepath.Join(to, "hello.txt")))
			_, statErr := os.Lstat(filepath.Join(to, "opaque"))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}
