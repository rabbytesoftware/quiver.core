package extract_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	stepextract "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/extract"
)

func TestHandler_Execute_DetectsEverySuffix(t *testing.T) {
	testCases := []struct {
		name  string
		file  string
		build func(t *testing.T) []byte
	}{
		{name: "tar.gz", file: "a.tar.gz", build: func(t *testing.T) []byte { return gzipBytes(t, helloTar(t)) }},
		{name: "tgz", file: "a.tgz", build: func(t *testing.T) []byte { return gzipBytes(t, helloTar(t)) }},
		{name: "tar.xz", file: "a.tar.xz", build: func(t *testing.T) []byte { return xzBytes(t, helloTar(t)) }},
		{name: "txz", file: "a.txz", build: func(t *testing.T) []byte { return xzBytes(t, helloTar(t)) }},
		{name: "tar.bz2", file: "a.tar.bz2", build: func(_ *testing.T) []byte { return bzip2TarFixture() }},
		{name: "tbz", file: "a.tbz", build: func(_ *testing.T) []byte { return bzip2TarFixture() }},
		{name: "tbz2", file: "a.tbz2", build: func(_ *testing.T) []byte { return bzip2TarFixture() }},
		{name: "tar.zst", file: "a.tar.zst", build: func(t *testing.T) []byte { return zstdBytes(t, helloTar(t)) }},
		{name: "tar", file: "a.tar", build: helloTar},
		{name: "zip", file: "a.zip", build: helloZip},
		{name: "upper case", file: "A.TAR.GZ", build: func(t *testing.T) []byte { return gzipBytes(t, helloTar(t)) }},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := writeArchive(t, dir, tc.file, tc.build(t))
			to := filepath.Join(dir, "out")

			require.NoError(t, runExtract(t, testMaxBytes, from, to, ""))

			data, err := os.ReadFile(filepath.Join(to, "hello.txt"))
			require.NoError(t, err)
			assert.Contains(t, string(data), "hello")
		})
	}
}

func TestHandler_Execute_FallsBackToMagicBytes(t *testing.T) {
	testCases := []struct {
		name     string
		build    func(t *testing.T) []byte
		wantFile string
		wantBody string
	}{
		{name: "tar", build: helloTar, wantFile: "hello.txt", wantBody: "hello\n"},
		{name: "zip", build: helloZip, wantFile: "hello.txt", wantBody: "hello\n"},
		{
			name:     "tar in gzip",
			build:    func(t *testing.T) []byte { return gzipBytes(t, helloTar(t)) },
			wantFile: "hello.txt",
			wantBody: "hello\n",
		},
		{
			name:     "tar in xz",
			build:    func(t *testing.T) []byte { return xzBytes(t, helloTar(t)) },
			wantFile: "hello.txt",
			wantBody: "hello\n",
		},
		{
			name:     "tar in zstd",
			build:    func(t *testing.T) []byte { return zstdBytes(t, helloTar(t)) },
			wantFile: "hello.txt",
			wantBody: "hello\n",
		},
		{
			name:     "tar in bzip2",
			build:    func(_ *testing.T) []byte { return bzip2TarFixture() },
			wantFile: "hello.txt",
			wantBody: "hello bzip2\n",
		},
		{
			name:     "single gzip",
			build:    func(t *testing.T) []byte { return gzipBytes(t, []byte("tool")) },
			wantFile: "payload",
			wantBody: "tool",
		},
		{
			name:     "single xz",
			build:    func(t *testing.T) []byte { return xzBytes(t, []byte("tool")) },
			wantFile: "payload",
			wantBody: "tool",
		},
		{
			name:     "single zstd",
			build:    func(t *testing.T) []byte { return zstdBytes(t, []byte("tool")) },
			wantFile: "payload",
			wantBody: "tool",
		},
		{
			name:     "single bzip2",
			build:    func(_ *testing.T) []byte { return bzip2PlainFixture() },
			wantFile: "payload",
			wantBody: "plain bzip2\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := writeArchive(t, dir, "payload.bin", tc.build(t))
			to := filepath.Join(dir, "out")

			require.NoError(t, runExtract(t, testMaxBytes, from, to, ""))

			assert.Equal(t, tc.wantBody, readString(t, filepath.Join(to, tc.wantFile)))
		})
	}
}

func TestHandler_Execute_UnknownFormat(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
	}{
		{name: "plain text", data: []byte("definitely not an archive")},
		{name: "empty", data: nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := writeArchive(t, dir, "payload.bin", tc.data)

			err := runExtract(t, testMaxBytes, from, filepath.Join(dir, "out"), "")

			assert.ErrorIs(t, err, stepextract.ErrUnknownFormat)
		})
	}
}

func TestHandler_Execute_CorruptCompressedMagic(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
	}{
		{name: "gzip", data: []byte{0x1f, 0x8b, 0x00, 0x00}},
		{name: "xz", data: []byte{0xfd, '7', 'z', 'X', 'Z', 0x00, 0xff, 0xff}},
		{name: "gzip body", data: []byte{0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xff}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := writeArchive(t, dir, "payload.bin", tc.data)

			err := runExtract(t, testMaxBytes, from, filepath.Join(dir, "out"), "")

			require.Error(t, err)
			assert.Contains(t, err.Error(), "extract:")
		})
	}
}

func TestHandler_Execute_DetectsDmgTrailer(t *testing.T) {
	dir := t.TempDir()
	data := make([]byte, 1024)
	copy(data[512:], "koly")
	from := writeArchive(t, dir, "image.bin", data)

	err := runExtract(t, testMaxBytes, from, filepath.Join(dir, "out"), "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "dmg")
}
