package unpacktest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const TestMaxBytes = int64(1 << 20)

type TarEntry struct {
	Name string
	Body string
	Mode int64
	Flag byte
	Link string
}

func HelloTar(
	t *testing.T,
) []byte {
	t.Helper()
	return TarBytes(t, TarEntry{Name: "hello.txt", Body: "hello\n", Mode: 0o644, Flag: tar.TypeReg})
}

func TarBytes(
	t *testing.T,
	entries ...TarEntry,
) []byte {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.Name,
			Mode:     e.Mode,
			Typeflag: e.Flag,
			Linkname: e.Link,
			Size:     int64(len(e.Body)),
			Format:   tar.FormatPAX,
		}
		require.NoError(t, tw.WriteHeader(hdr))
		_, err := tw.Write([]byte(e.Body))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())

	return buf.Bytes()
}

func GzipBytes(
	t *testing.T,
	data []byte,
) []byte {
	t.Helper()

	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, err := w.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	return buf.Bytes()
}

func WriteArchive(
	t *testing.T,
	dir string,
	name string,
	data []byte,
) string {
	t.Helper()

	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, data, 0o600))

	return path
}

func ReadString(
	t *testing.T,
	path string,
) string {
	t.Helper()

	data, err := os.ReadFile(path) //nolint:gosec
	require.NoError(t, err)

	return string(data)
}
