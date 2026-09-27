package unpack_test

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack/unpacktest"
)

func TestArchive_Extract_SizeCeiling(t *testing.T) {
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
				return unpacktest.TarBytes(t,
					unpacktest.TarEntry{Name: "one", Body: "123456", Mode: 0o644, Flag: tar.TypeReg},
					unpacktest.TarEntry{Name: "two", Body: "123456", Mode: 0o644, Flag: tar.TypeReg},
				)
			},
			maxBytes: 10,
			wantErr:  true,
		},
		{
			name: "cumulative tar entries exactly at the limit",
			file: "a.tar",
			build: func(t *testing.T) []byte {
				return unpacktest.TarBytes(t,
					unpacktest.TarEntry{Name: "one", Body: "12345", Mode: 0o644, Flag: tar.TypeReg},
					unpacktest.TarEntry{Name: "two", Body: "12345", Mode: 0o644, Flag: tar.TypeReg},
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
			build:    func(t *testing.T) []byte { return unpacktest.GzipBytes(t, []byte(strings.Repeat("0", 4096))) },
			maxBytes: 1024,
			wantErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := unpacktest.WriteArchive(t, dir, tc.file, tc.build(t))

			err := runUnpack(t, tc.maxBytes, from, filepath.Join(dir, "out"), 0)

			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, unpack.ErrTooLarge)
		})
	}
}

func TestGuard_EntryCeiling(t *testing.T) {
	entries := []unpacktest.TarEntry{
		{Name: "dir", Mode: 0o755, Flag: tar.TypeDir},
		{Name: "dir/file", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
		{Name: "dir/soft", Link: "file", Flag: tar.TypeSymlink},
		{Name: "dir/hard", Link: "dir/file", Flag: tar.TypeLink},
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

			err := unpack.ExtractTarWithEntryLimit(
				context.Background(), bytes.NewReader(unpacktest.TarBytes(t, entries...)), to, tc.limit,
			)

			if !tc.wantErr {
				require.NoError(t, err)
				assert.FileExists(t, filepath.Join(to, "dir", "hard"))
				return
			}
			require.ErrorIs(t, err, unpack.ErrTooMany)
			assert.Contains(t, err.Error(), "entries")
			_, statErr := os.Lstat(filepath.Join(to, filepath.FromSlash(tc.missing)))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func TestGuard_EntryCeilingIsOneMillion(t *testing.T) {
	assert.Equal(t, 1_000_000, unpack.MaxEntries)
}

func TestArchive_Extract_SizeCeilingRemovesPartialFile(t *testing.T) {
	dir := t.TempDir()
	from := unpacktest.WriteArchive(t, dir, "big.gz", unpacktest.GzipBytes(t, []byte(strings.Repeat("0", 4096))))
	to := filepath.Join(dir, "out")

	err := runUnpack(t, 1024, from, to, 0)

	require.ErrorIs(t, err, unpack.ErrTooLarge)
	_, statErr := os.Lstat(filepath.Join(to, "big"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestArchive_Extract_HonoursTimeout(t *testing.T) {
	dir := t.TempDir()
	from := unpacktest.WriteArchive(t, dir, "tool.gz", unpacktest.GzipBytes(t, []byte("tool")))

	err := runUnpack(t, unpacktest.TestMaxBytes, from, filepath.Join(dir, "out"), time.Nanosecond)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "deadline exceeded")
}

func TestArchive_Extract_RejectsLinksThatEscapeOnDisk(t *testing.T) {
	testCases := []struct {
		name     string
		entries  []unpacktest.TarEntry
		leftover string
	}{
		{
			name: "link placed through an in-archive symlink",
			entries: []unpacktest.TarEntry{
				{Name: "d/", Mode: 0o755, Flag: tar.TypeDir},
				{Name: "d/up", Mode: 0o777, Flag: tar.TypeSymlink, Link: ".."},
				{Name: "d/up/esc", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../outside"},
			},
			leftover: "esc",
		},
		{
			name: "dangling link through a symlink chain",
			entries: []unpacktest.TarEntry{
				{Name: "a/b/c/", Mode: 0o755, Flag: tar.TypeDir},
				{Name: "a/b/c/l", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../../.."},
				{Name: "y", Mode: 0o777, Flag: tar.TypeSymlink, Link: "a/b/c/l/../../../../outside/nd"},
			},
			leftover: "y",
		},
		{
			name: "symlink loop",
			entries: []unpacktest.TarEntry{
				{Name: "loop", Mode: 0o777, Flag: tar.TypeSymlink, Link: "loop"},
			},
			leftover: "loop",
		},
		{
			name: "link target traversing an in-archive symlink",
			entries: []unpacktest.TarEntry{
				{Name: "self", Mode: 0o777, Flag: tar.TypeSymlink, Link: "."},
				{Name: "esc", Mode: 0o777, Flag: tar.TypeSymlink, Link: "self/.."},
			},
			leftover: "esc",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "outside"), []byte("secret"), 0o600))
			from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.TarBytes(t, tc.entries...))
			to := filepath.Join(dir, "out")

			err := runUnpack(t, unpacktest.TestMaxBytes, from, to, 0)

			require.ErrorIs(t, err, unpack.ErrEscape)
			_, statErr := os.Lstat(filepath.Join(to, tc.leftover))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func TestArchive_Extract_RefusesToWriteThroughEscapingSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	to := filepath.Join(dir, "out")
	require.NoError(t, os.MkdirAll(to, 0o755))
	require.NoError(t, os.Symlink(outside, filepath.Join(to, "link")))
	from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.TarBytes(t,
		unpacktest.TarEntry{Name: "link/pwn.txt", Body: "pwn", Mode: 0o644, Flag: tar.TypeReg},
	))

	err := runUnpack(t, unpacktest.TestMaxBytes, from, to, 0)

	require.Error(t, err)
	_, statErr := os.Lstat(filepath.Join(outside, "pwn.txt"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestArchive_Extract_KeepsDanglingLinkInsideDestination(t *testing.T) {
	dir := t.TempDir()
	from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.TarBytes(t,
		unpacktest.TarEntry{Name: "lib/current", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../versions/missing"},
	))
	to := filepath.Join(dir, "out")

	require.NoError(t, runUnpack(t, unpacktest.TestMaxBytes, from, to, 0))

	target, err := os.Readlink(filepath.Join(to, "lib", "current"))
	require.NoError(t, err)
	assert.Equal(t, "../versions/missing", target)
}

func TestGuard_Verify_RemovesEveryEscapingLink(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "outside"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "outside", "secret"), []byte("secret"), 0o600))
	from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.TarBytes(t,
		unpacktest.TarEntry{Name: "a/b/c/", Mode: 0o755, Flag: tar.TypeDir},
		unpacktest.TarEntry{Name: "a/b/c/l", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../../.."},
		unpacktest.TarEntry{Name: "e1", Mode: 0o777, Flag: tar.TypeSymlink, Link: "a/b/c/l/../../../../outside"},
		unpacktest.TarEntry{Name: "e2", Mode: 0o777, Flag: tar.TypeSymlink, Link: "a/b/c/l/../../../../outside/secret"},
	))
	to := filepath.Join(dir, "deep", "out")

	err := runUnpack(t, unpacktest.TestMaxBytes, from, to, 0)

	require.ErrorIs(t, err, unpack.ErrEscape)
	assert.Contains(t, err.Error(), "e1")
	assert.Contains(t, err.Error(), "e2")
	for _, name := range []string{"e1", "e2"} {
		_, statErr := os.Lstat(filepath.Join(to, name))
		assert.ErrorIs(t, statErr, os.ErrNotExist, name)
	}
}

func TestGuard_Dest_ReturnsResolvedDestination(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out")

	g, err := unpack.OpenGuard(context.Background(), dest, unpacktest.TestMaxBytes)
	require.NoError(t, err)
	defer g.Close()

	resolved, err := filepath.EvalSymlinks(dest)
	require.NoError(t, err)
	assert.Equal(t, resolved, g.Dest())
}

func TestGuard_OpenGuard_AppliesOptions(t *testing.T) {
	dir := t.TempDir()
	called := false
	opt := func(*unpack.Guard) { called = true }

	g, err := unpack.OpenGuard(context.Background(), filepath.Join(dir, "out"), unpacktest.TestMaxBytes, opt)
	require.NoError(t, err)
	defer g.Close()

	assert.True(t, called)
}

func TestGuard_TopLevel_ReturnsSortedUniqueNames(t *testing.T) {
	dir := t.TempDir()
	from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.TarBytes(t,
		unpacktest.TarEntry{Name: "b/x", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
		unpacktest.TarEntry{Name: "a/y", Body: "y", Mode: 0o644, Flag: tar.TypeReg},
		unpacktest.TarEntry{Name: "a/z", Body: "z", Mode: 0o644, Flag: tar.TypeReg},
		unpacktest.TarEntry{Name: "top.txt", Body: "t", Mode: 0o644, Flag: tar.TypeReg},
	))
	to := filepath.Join(dir, "out")

	src, err := os.Open(from)
	require.NoError(t, err)
	defer src.Close()
	info, err := src.Stat()
	require.NoError(t, err)
	archive, err := unpack.DetectArchive(src, info.Size())
	require.NoError(t, err)

	g, err := unpack.OpenGuard(context.Background(), to, unpacktest.TestMaxBytes)
	require.NoError(t, err)
	defer g.Close()
	require.NoError(t, archive.Extract(context.Background(), src, info.Size(), g))

	assert.Equal(t, []string{"a", "b", "top.txt"}, g.TopLevel())
}

func TestArchive_Extract_WriteThroughArchiveEscapingSymlinkIsRefused(t *testing.T) {
	dir := t.TempDir()
	from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.TarBytes(t,
		unpacktest.TarEntry{Name: "self", Mode: 0o777, Flag: tar.TypeSymlink, Link: "."},
		unpacktest.TarEntry{Name: "esc", Mode: 0o777, Flag: tar.TypeSymlink, Link: "self/.."},
		unpacktest.TarEntry{Name: "esc/pwn.txt", Body: "pwn", Mode: 0o644, Flag: tar.TypeReg},
	))
	to := filepath.Join(dir, "out")

	err := runUnpack(t, unpacktest.TestMaxBytes, from, to, 0)

	require.Error(t, err)
	assert.ErrorIs(t, err, unpack.ErrEscape)
	_, statErr := os.Lstat(filepath.Join(dir, "pwn.txt"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
	_, statErr = os.Lstat(filepath.Join(to, "esc"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}
