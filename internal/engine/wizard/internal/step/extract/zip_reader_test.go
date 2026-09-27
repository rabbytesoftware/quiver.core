package extract_test

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	stepextract "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/extract"
)

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

func TestHandler_Execute_ZipPreservesLayoutAndModes(t *testing.T) {
	dir := t.TempDir()
	from := writeArchive(t, dir, "a.zip", zipBytes(t,
		zipEntry{name: "pkg/", mode: os.ModeDir | 0o755},
		zipEntry{name: "pkg/bin/tool", body: "#!/bin/sh\n", mode: 0o755},
		zipEntry{name: "pkg/README", body: "docs", mode: 0o644},
		zipEntry{name: "pkg/current", body: "bin/tool", mode: os.ModeSymlink | 0o777},
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
}

func TestHandler_Execute_ZipRejectsEscapes(t *testing.T) {
	testCases := []struct {
		name  string
		entry zipEntry
	}{
		{name: "parent traversal", entry: zipEntry{name: "../x", body: "pwn", mode: 0o644}},
		{name: "absolute path", entry: zipEntry{name: "/x", body: "pwn", mode: 0o644}},
		{name: "absolute symlink", entry: zipEntry{name: "x", body: "/etc", mode: os.ModeSymlink | 0o777}},
		{name: "relative symlink", entry: zipEntry{name: "x", body: "../../x", mode: os.ModeSymlink | 0o777}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := writeArchive(t, dir, "a.zip", zipBytes(t, tc.entry))
			to := filepath.Join(dir, "deep", "out")

			err := runExtract(t, testMaxBytes, from, to, "")

			require.ErrorIs(t, err, stepextract.ErrEscape)
			_, statErr := os.Lstat(filepath.Join(dir, "deep", "x"))
			assert.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func TestHandler_Execute_ZipFailures(t *testing.T) {
	testCases := []struct {
		name  string
		build func(t *testing.T) []byte
	}{
		{name: "not a zip", build: func(_ *testing.T) []byte { return []byte("PK\x03\x04 broken") }},
		{name: "unsupported method", build: func(t *testing.T) []byte { return rawZip(t, "x", 0o644, 99, "abc") }},
		{name: "bad file checksum", build: func(t *testing.T) []byte { return rawZip(t, "x", 0o644, zip.Store, "abc") }},
		{
			name:  "bad symlink checksum",
			build: func(t *testing.T) []byte { return rawZip(t, "x", os.ModeSymlink|0o777, zip.Store, "abc") },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := writeArchive(t, dir, "a.zip", tc.build(t))

			err := runExtract(t, testMaxBytes, from, filepath.Join(dir, "out"), "")

			require.Error(t, err)
			assert.Contains(t, err.Error(), "zip:")
		})
	}
}

func TestHandler_Execute_ZipHonoursTimeout(t *testing.T) {
	dir := t.TempDir()
	from := writeArchive(t, dir, "a.zip", helloZip(t))

	err := runExtract(t, testMaxBytes, from, filepath.Join(dir, "out"), "1ns")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "deadline exceeded")
}
