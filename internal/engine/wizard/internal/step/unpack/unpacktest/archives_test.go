package unpacktest_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack/unpacktest"
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
