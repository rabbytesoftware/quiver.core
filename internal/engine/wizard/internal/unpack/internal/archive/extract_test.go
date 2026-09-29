package archive_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/archive"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/guard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/unpacktest"
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

			require.ErrorIs(t, err, models.ErrEscape)
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
		verify  func(t *testing.T, to string)
	}{
		{
			name:    "file named dot",
			entries: []unpacktest.TarEntry{{Name: ".", Body: "x", Mode: 0o644, Flag: tar.TypeReg}},
			verify:  assertOutDirEmpty,
		},
		{
			name:    "empty symlink target",
			entries: []unpacktest.TarEntry{{Name: "x", Mode: 0o777, Flag: tar.TypeSymlink, Link: ""}},
			verify:  assertOutDirEmpty,
		},
		{
			name:    "hardlink to root",
			entries: []unpacktest.TarEntry{{Name: "x", Mode: 0o644, Flag: tar.TypeLink, Link: "./"}},
			verify:  assertOutDirEmpty,
		},
		{
			name:    "hardlink to missing entry",
			entries: []unpacktest.TarEntry{{Name: "x", Mode: 0o644, Flag: tar.TypeLink, Link: "missing"}},
			verify:  assertOutDirEmpty,
		},
		{
			name: "hardlink to symlink",
			entries: []unpacktest.TarEntry{
				{Name: "s", Mode: 0o777, Flag: tar.TypeSymlink, Link: "hello.txt"},
				{Name: "x", Mode: 0o644, Flag: tar.TypeLink, Link: "s"},
			},
			verify: func(t *testing.T, to string) {
				t.Helper()
				target, err := os.Readlink(filepath.Join(to, "s"))
				require.NoError(t, err)
				assert.Equal(t, "hello.txt", target)
				_, statErr := os.Lstat(filepath.Join(to, "x"))
				assert.ErrorIs(t, statErr, os.ErrNotExist)
			},
		},
		{
			name: "file replacing non-empty directory",
			entries: []unpacktest.TarEntry{
				{Name: "d/child", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
				{Name: "d", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
			},
			verify: func(t *testing.T, to string) {
				t.Helper()
				assert.DirExists(t, filepath.Join(to, "d"))
				assert.Equal(t, "x", unpacktest.ReadString(t, filepath.Join(to, "d", "child")))
			},
		},
		{
			name: "directory replacing file",
			entries: []unpacktest.TarEntry{
				{Name: "d", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
				{Name: "d/", Mode: 0o755, Flag: tar.TypeDir},
			},
			verify: func(t *testing.T, to string) {
				t.Helper()
				assert.Equal(t, "x", unpacktest.ReadString(t, filepath.Join(to, "d")))
			},
		},
		{
			name: "symlink replacing non-empty directory",
			entries: []unpacktest.TarEntry{
				{Name: "d/child", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
				{Name: "d", Mode: 0o777, Flag: tar.TypeSymlink, Link: "child"},
			},
			verify: func(t *testing.T, to string) {
				t.Helper()
				assert.DirExists(t, filepath.Join(to, "d"))
				assert.Equal(t, "x", unpacktest.ReadString(t, filepath.Join(to, "d", "child")))
			},
		},
		{
			name: "symlink under a file",
			entries: []unpacktest.TarEntry{
				{Name: "d", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
				{Name: "d/l", Mode: 0o777, Flag: tar.TypeSymlink, Link: "x"},
			},
			verify: assertDFileUntouchedNoChild,
		},
		{
			name: "hardlink under a file",
			entries: []unpacktest.TarEntry{
				{Name: "d", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
				{Name: "d/l", Mode: 0o644, Flag: tar.TypeLink, Link: "d"},
			},
			verify: assertDFileUntouchedNoChild,
		},
		{
			name: "hardlink to directory",
			entries: []unpacktest.TarEntry{
				{Name: "sub/", Mode: 0o755, Flag: tar.TypeDir},
				{Name: "x", Mode: 0o644, Flag: tar.TypeLink, Link: "sub"},
			},
			verify: func(t *testing.T, to string) {
				t.Helper()
				assert.DirExists(t, filepath.Join(to, "sub"))
				_, statErr := os.Lstat(filepath.Join(to, "x"))
				assert.ErrorIs(t, statErr, os.ErrNotExist)
			},
		},
		{
			name: "file under a file",
			entries: []unpacktest.TarEntry{
				{Name: "d", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
				{Name: "d/child", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
			},
			verify: assertDFileUntouchedNoChild,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.TarBytes(t, tc.entries...))
			to := filepath.Join(dir, "out")

			err := runUnpack(t, unpacktest.TestMaxBytes, from, to, 0)

			require.Error(t, err)
			tc.verify(t, to)
		})
	}
}

func assertOutDirEmpty(
	t *testing.T,
	to string,
) {
	t.Helper()

	entries, err := os.ReadDir(to)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func assertDFileUntouchedNoChild(
	t *testing.T,
	to string,
) {
	t.Helper()

	assert.Equal(t, "x", unpacktest.ReadString(t, filepath.Join(to, "d")))
	_, statErr := os.Lstat(filepath.Join(to, "d", "child"))
	assert.Error(t, statErr)
	_, statErr = os.Lstat(filepath.Join(to, "d", "l"))
	assert.Error(t, statErr)
}

func TestArchive_Extract_TarTruncated(t *testing.T) {
	dir := t.TempDir()
	from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.HelloTar(t)[:515])

	err := runUnpack(t, unpacktest.TestMaxBytes, from, filepath.Join(dir, "out"), 0)

	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

func TestArchive_Extract_TarSkippedBodiesCountTowardsCeiling(t *testing.T) {
	testCases := []struct {
		name     string
		maxBytes int64
		wantErr  error
	}{
		{name: "within ceiling", maxBytes: unpacktest.TestMaxBytes},
		{name: "over ceiling", maxBytes: 1024, wantErr: models.ErrTooLarge},
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

func rawZip(
	t *testing.T,
	name string,
	mode os.FileMode,
	method uint16,
	body string,
) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	hdr := &zip.FileHeader{
		Name:               name,
		Method:             method,
		CRC32:              0xdeadbeef,
		CompressedSize64:   uint64(len(body)),
		UncompressedSize64: uint64(len(body)),
	}
	hdr.SetMode(mode)
	w, err := zw.CreateRaw(hdr)
	require.NoError(t, err)
	_, err = w.Write([]byte(body))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	return buf.Bytes()
}

func TestArchive_Extract_ZipPreservesLayoutAndModes(t *testing.T) {
	dir := t.TempDir()
	from := unpacktest.WriteArchive(t, dir, "a.zip", unpacktest.ZipBytes(t,
		unpacktest.ZipEntry{Name: "pkg/", Mode: os.ModeDir | 0o755},
		unpacktest.ZipEntry{Name: "pkg/bin/tool", Body: "#!/bin/sh\n", Mode: 0o755},
		unpacktest.ZipEntry{Name: "pkg/README", Body: "docs", Mode: 0o644},
		unpacktest.ZipEntry{Name: "pkg/current", Body: "bin/tool", Mode: os.ModeSymlink | 0o777},
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
}

func TestArchive_Extract_ZipRejectsEscapes(t *testing.T) {
	testCases := []struct {
		name  string
		entry unpacktest.ZipEntry
	}{
		{name: "parent traversal", entry: unpacktest.ZipEntry{Name: "../x", Body: "pwn", Mode: 0o644}},
		{name: "absolute path", entry: unpacktest.ZipEntry{Name: "/x", Body: "pwn", Mode: 0o644}},
		{name: "absolute symlink", entry: unpacktest.ZipEntry{Name: "x", Body: "/etc", Mode: os.ModeSymlink | 0o777}},
		{name: "relative symlink", entry: unpacktest.ZipEntry{Name: "x", Body: "../../x", Mode: os.ModeSymlink | 0o777}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := unpacktest.WriteArchive(t, dir, "a.zip", unpacktest.ZipBytes(t, tc.entry))
			to := filepath.Join(dir, "deep", "out")

			err := runUnpack(t, unpacktest.TestMaxBytes, from, to, 0)

			require.ErrorIs(t, err, models.ErrEscape)
			_, statErr := os.Lstat(filepath.Join(dir, "deep", "x"))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func TestArchive_Extract_ZipFailures(t *testing.T) {
	testCases := []struct {
		name    string
		build   func(t *testing.T) []byte
		wantErr error
	}{
		{
			name:    "not a zip",
			build:   func(_ *testing.T) []byte { return []byte("PK\x03\x04 broken") },
			wantErr: zip.ErrFormat,
		},
		{
			name:    "unsupported method",
			build:   func(t *testing.T) []byte { return rawZip(t, "x", 0o644, 99, "abc") },
			wantErr: zip.ErrAlgorithm,
		},
		{
			name:    "bad file checksum",
			build:   func(t *testing.T) []byte { return rawZip(t, "x", 0o644, zip.Store, "abc") },
			wantErr: zip.ErrChecksum,
		},
		{
			name:    "bad symlink checksum",
			build:   func(t *testing.T) []byte { return rawZip(t, "x", os.ModeSymlink|0o777, zip.Store, "abc") },
			wantErr: zip.ErrChecksum,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := unpacktest.WriteArchive(t, dir, "a.zip", tc.build(t))

			err := runUnpack(t, unpacktest.TestMaxBytes, from, filepath.Join(dir, "out"), 0)

			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestArchive_Extract_ZipHonoursTimeout(t *testing.T) {
	dir := t.TempDir()
	from := unpacktest.WriteArchive(t, dir, "a.zip", helloZip(t))

	err := runUnpack(t, unpacktest.TestMaxBytes, from, filepath.Join(dir, "out"), time.Nanosecond)

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestArchive_Extract_SingleFileStripsExtension(t *testing.T) {
	testCases := []struct {
		name     string
		file     string
		build    func(t *testing.T) []byte
		wantFile string
		wantBody string
	}{
		{
			name:     "gzip",
			file:     "tool.gz",
			build:    func(t *testing.T) []byte { return unpacktest.GzipBytes(t, []byte("gz tool")) },
			wantFile: "tool",
			wantBody: "gz tool",
		},
		{
			name:     "xz",
			file:     "tool-linux.xz",
			build:    func(t *testing.T) []byte { return xzBytes(t, []byte("xz tool")) },
			wantFile: "tool-linux",
			wantBody: "xz tool",
		},
		{
			name:     "zstd",
			file:     "tool.v2.zst",
			build:    func(t *testing.T) []byte { return zstdBytes(t, []byte("zst tool")) },
			wantFile: "tool.v2",
			wantBody: "zst tool",
		},
		{
			name:     "bzip2",
			file:     "plain.bz2",
			build:    func(_ *testing.T) []byte { return bzip2PlainFixture() },
			wantFile: "plain",
			wantBody: "plain bzip2\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := unpacktest.WriteArchive(t, dir, tc.file, tc.build(t))
			to := filepath.Join(dir, "bin")

			require.NoError(t, runUnpack(t, unpacktest.TestMaxBytes, from, to, 0))

			out := filepath.Join(to, tc.wantFile)
			assert.Equal(t, tc.wantBody, unpacktest.ReadString(t, out))
			info, err := os.Stat(out)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
		})
	}
}

func TestArchive_Extract_SingleFileWithoutStem(t *testing.T) {
	dir := t.TempDir()
	from := unpacktest.WriteArchive(t, dir, ".gz", unpacktest.GzipBytes(t, []byte("x")))
	to := filepath.Join(dir, "out")

	err := runUnpack(t, unpacktest.TestMaxBytes, from, to, 0)

	require.Error(t, err)
	entries, rdErr := os.ReadDir(to)
	require.NoError(t, rdErr)
	assert.Empty(t, entries)
}

func TestArchive_ExtractAs_NamesTheSingleFile(t *testing.T) {
	testCases := []struct {
		name     string
		file     string
		build    func(t *testing.T) []byte
		as       string
		wantFile string
	}{
		{name: "single file takes the name", file: ".tool.download", build: func(t *testing.T) []byte { return unpacktest.GzipBytes(t, []byte("bin")) }, as: "tool", wantFile: "tool"},
		{name: "empty name keeps the stem", file: "tool-linux.gz", build: func(t *testing.T) []byte { return unpacktest.GzipBytes(t, []byte("bin")) }, wantFile: "tool-linux"},
		{name: "tar ignores the name", file: "tool.tar", build: func(t *testing.T) []byte {
			return unpacktest.TarBytes(t, unpacktest.TarEntry{Name: "inner", Body: "bin", Mode: 0o755, Flag: tar.TypeReg})
		}, as: "tool", wantFile: "inner"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := unpacktest.WriteArchive(t, dir, tc.file, tc.build(t))
			to := filepath.Join(dir, "bin")
			src, err := os.Open(from)
			require.NoError(t, err)
			defer src.Close()
			info, err := src.Stat()
			require.NoError(t, err)
			kind, err := archive.Detect(src, info.Size())
			require.NoError(t, err)
			g, err := guard.Open(context.Background(), to, unpacktest.TestMaxBytes)
			require.NoError(t, err)
			defer g.Close()

			require.NoError(t, kind.ExtractAs(context.Background(), src, info.Size(), g, tc.as))

			assert.Equal(t, "bin", unpacktest.ReadString(t, filepath.Join(to, tc.wantFile)))
		})
	}
}
