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

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

func TestArchive_Extract_TarPreservesLayoutAndModes(t *testing.T) {
	dir := t.TempDir()
	from := mocks.WriteFile(t, filepath.Join(dir, "a.tar"), mocks.TarBytes(t,
		mocks.TarEntry{Name: "./", Mode: 0o755, Flag: tar.TypeDir},
		mocks.TarEntry{Name: "pkg/", Mode: 0o755, Flag: tar.TypeDir},
		mocks.TarEntry{Name: "pkg/bin/tool", Body: "#!/bin/sh\n", Mode: 0o755, Flag: tar.TypeReg},
		mocks.TarEntry{Name: "pkg/README", Body: "docs", Mode: 0o644, Flag: tar.TypeReg},
		mocks.TarEntry{Name: "pkg/current", Mode: 0o777, Flag: tar.TypeSymlink, Link: "bin/tool"},
		mocks.TarEntry{Name: "pkg/tool-hard", Mode: 0o755, Flag: tar.TypeLink, Link: "pkg/bin/tool"},
		mocks.TarEntry{Name: "pkg/fifo", Mode: 0o644, Flag: tar.TypeFifo},
	))
	to := filepath.Join(dir, "out")

	require.NoError(t, runUnpack(t, mocks.TestMaxBytes, from, to, 0))

	tool, err := os.Stat(filepath.Join(to, "pkg", "bin", "tool"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), tool.Mode().Perm())

	readme, err := os.Stat(filepath.Join(to, "pkg", "README"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), readme.Mode().Perm())

	target, err := os.Readlink(filepath.Join(to, "pkg", "current"))
	require.NoError(t, err)
	assert.Equal(t, "bin/tool", target)

	assert.Equal(t, "#!/bin/sh\n", mocks.ReadString(t, filepath.Join(to, "pkg", "tool-hard")))

	_, err = os.Lstat(filepath.Join(to, "pkg", "fifo"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestArchive_Extract_TarRejectsEscapes(t *testing.T) {
	testCases := []struct {
		name  string
		entry mocks.TarEntry
	}{
		{name: "parent traversal", entry: mocks.TarEntry{Name: "../x", Body: "pwn", Mode: 0o644, Flag: tar.TypeReg}},
		{
			name:  "nested traversal",
			entry: mocks.TarEntry{Name: "a/../../x", Body: "pwn", Mode: 0o644, Flag: tar.TypeReg},
		},
		{name: "absolute path", entry: mocks.TarEntry{Name: "/x", Body: "pwn", Mode: 0o644, Flag: tar.TypeReg}},
		{name: "traversal dir", entry: mocks.TarEntry{Name: "../x/", Mode: 0o755, Flag: tar.TypeDir}},
		{
			name:  "absolute symlink",
			entry: mocks.TarEntry{Name: "x", Mode: 0o777, Flag: tar.TypeSymlink, Link: "/etc"},
		},
		{
			name:  "relative symlink",
			entry: mocks.TarEntry{Name: "a/x", Mode: 0o777, Flag: tar.TypeSymlink, Link: "../../x"},
		},
		{
			name:  "symlink named outside",
			entry: mocks.TarEntry{Name: "../x", Mode: 0o777, Flag: tar.TypeSymlink, Link: "y"},
		},
		{
			name:  "relative hardlink",
			entry: mocks.TarEntry{Name: "x", Mode: 0o644, Flag: tar.TypeLink, Link: "../x"},
		},
		{
			name:  "absolute hardlink",
			entry: mocks.TarEntry{Name: "x", Mode: 0o644, Flag: tar.TypeLink, Link: "/etc/passwd"},
		},
		{
			name:  "hardlink named outside",
			entry: mocks.TarEntry{Name: "../x", Mode: 0o644, Flag: tar.TypeLink, Link: "y"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := mocks.WriteFile(t, filepath.Join(dir, "a.tar"), mocks.TarBytes(t, tc.entry))
			to := filepath.Join(dir, "deep", "out")

			err := runUnpack(t, mocks.TestMaxBytes, from, to, 0)

			require.ErrorIs(t, err, models.ErrEscape)
			_, statErr := os.Lstat(filepath.Join(dir, "deep", "x"))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
			_, statErr = os.Lstat(filepath.Join(dir, "x"))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func TestArchive_Extract_TarTruncated(t *testing.T) {
	dir := t.TempDir()
	from := mocks.WriteFile(t, filepath.Join(dir, "a.tar"), mocks.HelloTar(t)[:515])

	err := runUnpack(t, mocks.TestMaxBytes, from, filepath.Join(dir, "out"), 0)

	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

func TestArchive_Extract_TarSkippedBodiesCountTowardsCeiling(t *testing.T) {
	testCases := []struct {
		name     string
		maxBytes int64
		wantErr  error
	}{
		{name: "within ceiling", maxBytes: mocks.TestMaxBytes},
		{name: "over ceiling", maxBytes: 1024, wantErr: models.ErrTooLarge},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := mocks.WriteFile(t, filepath.Join(dir, "a.tar"), mocks.TarBytes(t,
				mocks.TarEntry{Name: "opaque", Body: strings.Repeat("0", 4096), Mode: 0o644, Flag: 'Z'},
				mocks.TarEntry{Name: "hello.txt", Body: "hello\n", Mode: 0o644, Flag: tar.TypeReg},
			))
			to := filepath.Join(dir, "out")

			err := runUnpack(t, tc.maxBytes, from, to, 0)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "hello\n", mocks.ReadString(t, filepath.Join(to, "hello.txt")))
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
	from := mocks.WriteFile(t, filepath.Join(dir, "a.zip"), mocks.ZipBytes(t,
		mocks.ZipEntry{Name: "pkg/", Mode: os.ModeDir | 0o755},
		mocks.ZipEntry{Name: "pkg/bin/tool", Body: "#!/bin/sh\n", Mode: 0o755},
		mocks.ZipEntry{Name: "pkg/README", Body: "docs", Mode: 0o644},
		mocks.ZipEntry{Name: "pkg/current", Body: "bin/tool", Mode: os.ModeSymlink | 0o777},
	))
	to := filepath.Join(dir, "out")

	require.NoError(t, runUnpack(t, mocks.TestMaxBytes, from, to, 0))

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
		entry mocks.ZipEntry
	}{
		{name: "parent traversal", entry: mocks.ZipEntry{Name: "../x", Body: "pwn", Mode: 0o644}},
		{name: "absolute path", entry: mocks.ZipEntry{Name: "/x", Body: "pwn", Mode: 0o644}},
		{name: "absolute symlink", entry: mocks.ZipEntry{Name: "x", Body: "/etc", Mode: os.ModeSymlink | 0o777}},
		{name: "relative symlink", entry: mocks.ZipEntry{Name: "x", Body: "../../x", Mode: os.ModeSymlink | 0o777}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := mocks.WriteFile(t, filepath.Join(dir, "a.zip"), mocks.ZipBytes(t, tc.entry))
			to := filepath.Join(dir, "deep", "out")

			err := runUnpack(t, mocks.TestMaxBytes, from, to, 0)

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
			from := mocks.WriteFile(t, filepath.Join(dir, "a.zip"), tc.build(t))

			err := runUnpack(t, mocks.TestMaxBytes, from, filepath.Join(dir, "out"), 0)

			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestArchive_Extract_ZipHonoursTimeout(t *testing.T) {
	dir := t.TempDir()
	from := mocks.WriteFile(t, filepath.Join(dir, "a.zip"), helloZip(t))

	err := runUnpack(t, mocks.TestMaxBytes, from, filepath.Join(dir, "out"), time.Nanosecond)

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestArchive_Extract_NamesTheSingleFile(t *testing.T) {
	testCases := []struct {
		name     string
		file     string
		build    func(t *testing.T) []byte
		as       string
		wantFile string
	}{
		{name: "single file takes the name", file: ".tool.download", build: func(t *testing.T) []byte { return mocks.GzipBytes(t, []byte("bin")) }, as: "tool", wantFile: "tool"},
		{name: "empty name keeps the stem", file: "tool-linux.gz", build: func(t *testing.T) []byte { return mocks.GzipBytes(t, []byte("bin")) }, wantFile: "tool-linux"},
		{name: "tar ignores the name", file: "tool.tar", build: func(t *testing.T) []byte {
			return mocks.TarBytes(t, mocks.TarEntry{Name: "inner", Body: "bin", Mode: 0o755, Flag: tar.TypeReg})
		}, as: "tool", wantFile: "inner"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := mocks.WriteFile(t, filepath.Join(dir, tc.file), tc.build(t))
			to := filepath.Join(dir, "bin")

			_, err := unpackNamed(t, mocks.TestMaxBytes, from, to, 0, tc.as)

			require.NoError(t, err)
			assert.Equal(t, "bin", mocks.ReadString(t, filepath.Join(to, tc.wantFile)))
		})
	}
}
