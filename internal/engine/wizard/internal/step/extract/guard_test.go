package extract_test

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	stepextract "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/extract"
)

func TestHandler_Execute_SizeCeiling(t *testing.T) {
	testCases := []struct {
		name     string
		file     string
		build    func(t *testing.T) []byte
		maxBytes int64
		wantErr  bool
	}{
		{
			name: "cumulative tar entries over the limit",
			file: "a.tar",
			build: func(t *testing.T) []byte {
				return tarBytes(t,
					tarEntry{name: "one", body: "123456", mode: 0o644, flag: tar.TypeReg},
					tarEntry{name: "two", body: "123456", mode: 0o644, flag: tar.TypeReg},
				)
			},
			maxBytes: 10,
			wantErr:  true,
		},
		{
			name: "cumulative tar entries exactly at the limit",
			file: "a.tar",
			build: func(t *testing.T) []byte {
				return tarBytes(t,
					tarEntry{name: "one", body: "12345", mode: 0o644, flag: tar.TypeReg},
					tarEntry{name: "two", body: "12345", mode: 0o644, flag: tar.TypeReg},
				)
			},
			maxBytes: 10,
		},
		{
			name: "zip bomb",
			file: "a.zip",
			build: func(t *testing.T) []byte {
				return zipBytes(t, zipEntry{name: "big", body: strings.Repeat("0", 4096), mode: 0o644})
			},
			maxBytes: 1024,
			wantErr:  true,
		},
		{
			name:     "single file bomb",
			file:     "big.gz",
			build:    func(t *testing.T) []byte { return gzipBytes(t, []byte(strings.Repeat("0", 4096))) },
			maxBytes: 1024,
			wantErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := writeArchive(t, dir, tc.file, tc.build(t))

			err := runExtract(t, tc.maxBytes, from, filepath.Join(dir, "out"), "")

			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, stepextract.ErrTooLarge)
		})
	}
}

func TestGuard_EntryCeiling(t *testing.T) {
	entries := []tarEntry{
		{name: "dir", mode: 0o755, flag: tar.TypeDir},
		{name: "dir/file", body: "x", mode: 0o644, flag: tar.TypeReg},
		{name: "dir/soft", link: "file", flag: tar.TypeSymlink},
		{name: "dir/hard", link: "dir/file", flag: tar.TypeLink},
	}
	testCases := []struct {
		name    string
		limit   int
		wantErr bool
		missing string
	}{
		{name: "every entry kind within the limit extracts", limit: 4},
		{name: "one entry past the limit stops extraction", limit: 3, wantErr: true, missing: "dir/hard"},
		{name: "the limit stops before the first file", limit: 1, wantErr: true, missing: "dir/file"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			to := filepath.Join(t.TempDir(), "out")

			err := stepextract.ExtractTarWithEntryLimit(context.Background(), bytes.NewReader(tarBytes(t, entries...)), to, tc.limit)

			if !tc.wantErr {
				require.NoError(t, err)
				assert.FileExists(t, filepath.Join(to, "dir", "hard"))
				return
			}
			require.ErrorIs(t, err, stepextract.ErrTooMany)
			assert.Contains(t, err.Error(), "entries")
			_, statErr := os.Lstat(filepath.Join(to, filepath.FromSlash(tc.missing)))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func TestGuard_EntryCeilingIsOneMillion(t *testing.T) {
	assert.Equal(t, 1_000_000, stepextract.MaxEntries)
}

func TestHandler_Execute_SizeCeilingRemovesPartialFile(t *testing.T) {
	dir := t.TempDir()
	from := writeArchive(t, dir, "big.gz", gzipBytes(t, []byte(strings.Repeat("0", 4096))))
	to := filepath.Join(dir, "out")

	err := runExtract(t, 1024, from, to, "")

	require.ErrorIs(t, err, stepextract.ErrTooLarge)
	_, statErr := os.Lstat(filepath.Join(to, "big"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestHandler_Execute_SingleFileHonoursTimeout(t *testing.T) {
	dir := t.TempDir()
	from := writeArchive(t, dir, "tool.gz", gzipBytes(t, []byte("tool")))

	err := runExtract(t, testMaxBytes, from, filepath.Join(dir, "out"), "1ns")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "deadline exceeded")
}

func TestHandler_Execute_RejectsLinksThatEscapeOnDisk(t *testing.T) {
	testCases := []struct {
		name     string
		entries  []tarEntry
		leftover string
	}{
		{
			name: "link placed through an in-archive symlink",
			entries: []tarEntry{
				{name: "d/", mode: 0o755, flag: tar.TypeDir},
				{name: "d/up", mode: 0o777, flag: tar.TypeSymlink, link: ".."},
				{name: "d/up/esc", mode: 0o777, flag: tar.TypeSymlink, link: "../outside"},
			},
			leftover: "esc",
		},
		{
			name: "dangling link through a symlink chain",
			entries: []tarEntry{
				{name: "a/b/c/", mode: 0o755, flag: tar.TypeDir},
				{name: "a/b/c/l", mode: 0o777, flag: tar.TypeSymlink, link: "../../.."},
				{name: "y", mode: 0o777, flag: tar.TypeSymlink, link: "a/b/c/l/../../../../outside/nd"},
			},
			leftover: "y",
		},
		{
			name: "symlink loop",
			entries: []tarEntry{
				{name: "loop", mode: 0o777, flag: tar.TypeSymlink, link: "loop"},
			},
			leftover: "loop",
		},
		{
			name: "link target traversing an in-archive symlink",
			entries: []tarEntry{
				{name: "self", mode: 0o777, flag: tar.TypeSymlink, link: "."},
				{name: "esc", mode: 0o777, flag: tar.TypeSymlink, link: "self/.."},
			},
			leftover: "esc",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "outside"), []byte("secret"), 0o600))
			from := writeArchive(t, dir, "a.tar", tarBytes(t, tc.entries...))
			to := filepath.Join(dir, "out")

			err := runExtract(t, testMaxBytes, from, to, "")

			require.ErrorIs(t, err, stepextract.ErrEscape)
			_, statErr := os.Lstat(filepath.Join(to, tc.leftover))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func TestHandler_Execute_RefusesToWriteThroughEscapingSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	to := filepath.Join(dir, "out")
	require.NoError(t, os.MkdirAll(to, 0o755))
	require.NoError(t, os.Symlink(outside, filepath.Join(to, "link")))
	from := writeArchive(t, dir, "a.tar", tarBytes(t,
		tarEntry{name: "link/pwn.txt", body: "pwn", mode: 0o644, flag: tar.TypeReg},
	))

	err := runExtract(t, testMaxBytes, from, to, "")

	require.Error(t, err)
	_, statErr := os.Lstat(filepath.Join(outside, "pwn.txt"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestHandler_Execute_KeepsDanglingLinkInsideDestination(t *testing.T) {
	dir := t.TempDir()
	from := writeArchive(t, dir, "a.tar", tarBytes(t,
		tarEntry{name: "lib/current", mode: 0o777, flag: tar.TypeSymlink, link: "../versions/missing"},
	))
	to := filepath.Join(dir, "out")

	require.NoError(t, runExtract(t, testMaxBytes, from, to, ""))

	target, err := os.Readlink(filepath.Join(to, "lib", "current"))
	require.NoError(t, err)
	assert.Equal(t, "../versions/missing", target)
}

func TestHandler_Execute_RemovesEveryEscapingLink(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "outside"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "outside", "secret"), []byte("secret"), 0o600))
	from := writeArchive(t, dir, "a.tar", tarBytes(t,
		tarEntry{name: "a/b/c/", mode: 0o755, flag: tar.TypeDir},
		tarEntry{name: "a/b/c/l", mode: 0o777, flag: tar.TypeSymlink, link: "../../.."},
		tarEntry{name: "e1", mode: 0o777, flag: tar.TypeSymlink, link: "a/b/c/l/../../../../outside"},
		tarEntry{name: "e2", mode: 0o777, flag: tar.TypeSymlink, link: "a/b/c/l/../../../../outside/secret"},
	))
	to := filepath.Join(dir, "deep", "out")

	err := runExtract(t, testMaxBytes, from, to, "")

	require.ErrorIs(t, err, stepextract.ErrEscape)
	assert.Contains(t, err.Error(), "e1")
	assert.Contains(t, err.Error(), "e2")
	for _, name := range []string{"e1", "e2"} {
		_, statErr := os.Lstat(filepath.Join(to, name))
		assert.ErrorIs(t, statErr, os.ErrNotExist, name)
	}
}

func TestHandler_Execute_WriteThroughArchiveEscapingSymlinkIsRefused(t *testing.T) {
	dir := t.TempDir()
	from := writeArchive(t, dir, "a.tar", tarBytes(t,
		tarEntry{name: "self", mode: 0o777, flag: tar.TypeSymlink, link: "."},
		tarEntry{name: "esc", mode: 0o777, flag: tar.TypeSymlink, link: "self/.."},
		tarEntry{name: "esc/pwn.txt", body: "pwn", mode: 0o644, flag: tar.TypeReg},
	))
	to := filepath.Join(dir, "out")

	err := runExtract(t, testMaxBytes, from, to, "")

	require.Error(t, err)
	assert.ErrorIs(t, err, stepextract.ErrEscape)
	_, statErr := os.Lstat(filepath.Join(dir, "pwn.txt"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
	_, statErr = os.Lstat(filepath.Join(to, "esc"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}
