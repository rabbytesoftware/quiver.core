package unpacktest_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/unpacktest"
)

func TestTarBytes_WritesReadableEntries(t *testing.T) {
	data := unpacktest.TarBytes(t,
		unpacktest.TarEntry{Name: "a.txt", Body: "one", Mode: 0o644, Flag: tar.TypeReg},
		unpacktest.TarEntry{Name: "b.txt", Body: "two", Mode: 0o644, Flag: tar.TypeReg},
	)

	tr := tar.NewReader(bytes.NewReader(data))

	hdr, err := tr.Next()
	require.NoError(t, err)
	assert.Equal(t, "a.txt", hdr.Name)
	body, err := io.ReadAll(tr)
	require.NoError(t, err)
	assert.Equal(t, "one", string(body))

	hdr, err = tr.Next()
	require.NoError(t, err)
	assert.Equal(t, "b.txt", hdr.Name)
}

func TestHelloTar_ContainsHelloFile(t *testing.T) {
	data := unpacktest.HelloTar(t)

	tr := tar.NewReader(bytes.NewReader(data))
	hdr, err := tr.Next()
	require.NoError(t, err)
	assert.Equal(t, "hello.txt", hdr.Name)
	body, err := io.ReadAll(tr)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(body))
}

func TestZipBytes_WritesReadableEntries(t *testing.T) {
	data := unpacktest.ZipBytes(t,
		unpacktest.ZipEntry{Name: "dir/", Mode: os.ModeDir | 0o755},
		unpacktest.ZipEntry{Name: "dir/a.txt", Body: "one", Mode: 0o644},
	)

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	require.Len(t, zr.File, 2)
	assert.True(t, zr.File[0].Mode().IsDir())
	rc, err := zr.File[1].Open()
	require.NoError(t, err)
	defer rc.Close()
	body, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "one", string(body))
	assert.Equal(t, os.FileMode(0o644), zr.File[1].Mode().Perm())
}

func TestGzipBytes_WritesReadableStream(t *testing.T) {
	data := unpacktest.GzipBytes(t, []byte("payload"))

	gr, err := gzip.NewReader(bytes.NewReader(data))
	require.NoError(t, err)
	body, err := io.ReadAll(gr)
	require.NoError(t, err)
	assert.Equal(t, "payload", string(body))
}

func TestWriteArchive_WritesFileToDisk(t *testing.T) {
	dir := t.TempDir()

	path := unpacktest.WriteArchive(t, dir, "a.bin", []byte("data"))

	assert.Equal(t, filepath.Join(dir, "a.bin"), path)
	assert.Equal(t, "data", unpacktest.ReadString(t, path))
}

func TestReadString_ReturnsFileContents(t *testing.T) {
	dir := t.TempDir()
	path := unpacktest.WriteArchive(t, dir, "b.bin", []byte("hello world"))

	assert.Equal(t, "hello world", unpacktest.ReadString(t, path))
}

func TestTestMaxBytes_IsOneMebibyte(t *testing.T) {
	assert.Equal(t, int64(1<<20), unpacktest.TestMaxBytes)
}

func TestDmgTrailer_HasKolyMarkerOneBlockFromTheEnd(t *testing.T) {
	data := unpacktest.DmgTrailer()

	require.Len(t, data, 1024)
	assert.Equal(t, "koly", string(data[512:516]))
}

func TestZipFiles_WritesReadableEntries(t *testing.T) {
	data := unpacktest.ZipFiles(t, map[string]string{"a/b.txt": "one"})

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	require.Len(t, zr.File, 1)
	assert.Equal(t, "a/b.txt", zr.File[0].Name)
}

func TestWriteFile_CreatesParentDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "file")

	got := unpacktest.WriteFile(t, path, []byte("x"))

	assert.Equal(t, path, got)
	assert.Equal(t, "x", unpacktest.ReadString(t, path))
}
