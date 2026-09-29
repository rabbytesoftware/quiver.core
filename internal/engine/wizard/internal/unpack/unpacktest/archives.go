package unpacktest

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	TestMaxBytes  = int64(1 << 20)
	ElfExecutable = "\x7fELF\x02\x01\x01\x00binary"
)

type ZipEntry struct {
	Name string
	Body string
	Mode os.FileMode
}

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

func ZipBytes(
	t *testing.T,
	entries ...ZipEntry,
) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		hdr := &zip.FileHeader{Name: e.Name, Method: zip.Deflate}
		hdr.SetMode(e.Mode)
		w, err := zw.CreateHeader(hdr)
		require.NoError(t, err)
		_, err = w.Write([]byte(e.Body))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())

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

	return WriteFile(t, filepath.Join(dir, name), data)
}

func ReadString(
	t *testing.T,
	path string,
) string {
	t.Helper()

	data, err := os.ReadFile(path) // #nosec G304 -- test fixture in a temp dir
	require.NoError(t, err)

	return string(data)
}

func DmgTrailer() []byte {
	trailer := make([]byte, 1024)
	copy(trailer[512:], "koly")

	return trailer
}

func ZipFiles(
	t *testing.T,
	files map[string]string,
) []byte {
	t.Helper()

	entries := make([]ZipEntry, 0, len(files))
	for _, name := range slices.Sorted(maps.Keys(files)) {
		entries = append(entries, ZipEntry{Name: name, Body: files[name], Mode: 0o644})
	}

	return ZipBytes(t, entries...)
}

func WriteFile(
	t *testing.T,
	path string,
	data []byte,
) string {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755)) // #nosec G301 -- test fixture in a temp dir
	require.NoError(t, os.WriteFile(path, data, 0o600))        // #nosec G703 -- test fixture in a temp dir

	return path
}
