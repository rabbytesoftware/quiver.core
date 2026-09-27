package unpack_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ulikunitz/xz"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack/unpacktest"
)

type zipEntry struct {
	name string
	body string
	mode os.FileMode
}

func helloZip(
	t *testing.T,
) []byte {
	t.Helper()
	return zipBytes(t, zipEntry{name: "hello.txt", body: "hello\n", mode: 0o644})
}

func zipBytes(
	t *testing.T,
	entries ...zipEntry,
) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		hdr := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		hdr.SetMode(e.mode)
		w, err := zw.CreateHeader(hdr)
		require.NoError(t, err)
		_, err = w.Write([]byte(e.body))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())

	return buf.Bytes()
}

func xzBytes(
	t *testing.T,
	data []byte,
) []byte {
	t.Helper()

	var buf bytes.Buffer
	w, err := xz.NewWriter(&buf)
	require.NoError(t, err)
	_, err = w.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	return buf.Bytes()
}

func zstdBytes(
	t *testing.T,
	data []byte,
) []byte {
	t.Helper()

	var buf bytes.Buffer
	w, err := zstd.NewWriter(&buf)
	require.NoError(t, err)
	_, err = w.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	return buf.Bytes()
}

func bzip2TarFixture() []byte {
	return []byte{
		0x42, 0x5a, 0x68, 0x39, 0x31, 0x41, 0x59, 0x26, 0x53, 0x59, 0x8f, 0x5f, 0xca, 0xd2, 0x00, 0x00,
		0x3b, 0xdb, 0x90, 0xd2, 0x10, 0x40, 0x01, 0x7f, 0x04, 0x00, 0x80, 0x72, 0x64, 0xde, 0x50, 0x04,
		0x00, 0x02, 0x08, 0x20, 0x00, 0x75, 0x0d, 0x53, 0x1a, 0x8c, 0x83, 0x43, 0x6a, 0x19, 0x03, 0x4f,
		0x28, 0x24, 0x94, 0x62, 0x00, 0x00, 0x1a, 0x01, 0x08, 0xb4, 0x3e, 0x44, 0xcf, 0x04, 0x55, 0x17,
		0x84, 0xa2, 0x20, 0x8a, 0xe8, 0x2c, 0x61, 0xd2, 0x1c, 0x0f, 0x83, 0x21, 0xcc, 0x21, 0xb0, 0x64,
		0x6e, 0x29, 0x19, 0x93, 0x19, 0x8c, 0xae, 0xec, 0x9e, 0xfc, 0xa0, 0x1a, 0x4d, 0x85, 0xae, 0x18,
		0x58, 0x77, 0xf2, 0x11, 0x57, 0xc1, 0x50, 0x2b, 0xea, 0x2a, 0xfc, 0x99, 0xae, 0xa9, 0x7e, 0x2e,
		0xe4, 0x8a, 0x70, 0xa1, 0x21, 0x1e, 0xbf, 0x95, 0xa4,
	}
}

func bzip2PlainFixture() []byte {
	return []byte{
		0x42, 0x5a, 0x68, 0x39, 0x31, 0x41, 0x59, 0x26, 0x53, 0x59, 0x31, 0x0c, 0xf7, 0xa6, 0x00, 0x00,
		0x05, 0x59, 0x80, 0x00, 0x10, 0x40, 0x00, 0x10, 0x00, 0x30, 0x25, 0x40, 0x10, 0x20, 0x00, 0x22,
		0x00, 0x36, 0xa1, 0x00, 0x30, 0x9e, 0x4b, 0x34, 0xb1, 0x91, 0xe2, 0xee, 0x48, 0xa7, 0x0a, 0x12,
		0x06, 0x21, 0x9e, 0xf4, 0xc0,
	}
}

func dmgTrailerFixture() []byte {
	trailer := make([]byte, 1024)
	copy(trailer[512:], "koly")

	return trailer
}

func runUnpack(
	t *testing.T,
	maxBytes int64,
	from string,
	to string,
	timeout time.Duration,
) error {
	t.Helper()

	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	src, err := os.Open(from)
	require.NoError(t, err)
	defer src.Close()

	info, err := src.Stat()
	require.NoError(t, err)

	archive, err := unpack.DetectArchive(src, info.Size())
	if err != nil {
		return err
	}

	g, err := unpack.OpenGuard(context.Background(), to, maxBytes)
	if err != nil {
		return err
	}
	defer g.Close()

	return errors.Join(archive.Extract(ctx, src, info.Size(), g), g.Verify())
}

func TestDetectArchive_DetectsEverySuffix(t *testing.T) {
	testCases := []struct {
		name  string
		file  string
		build func(t *testing.T) []byte
	}{
		{
			name: "tar.gz", file: "a.tar.gz",
			build: func(t *testing.T) []byte { return unpacktest.GzipBytes(t, unpacktest.HelloTar(t)) },
		},
		{
			name: "tgz", file: "a.tgz",
			build: func(t *testing.T) []byte { return unpacktest.GzipBytes(t, unpacktest.HelloTar(t)) },
		},
		{name: "tar.xz", file: "a.tar.xz", build: func(t *testing.T) []byte { return xzBytes(t, unpacktest.HelloTar(t)) }},
		{name: "txz", file: "a.txz", build: func(t *testing.T) []byte { return xzBytes(t, unpacktest.HelloTar(t)) }},
		{name: "tar.bz2", file: "a.tar.bz2", build: func(_ *testing.T) []byte { return bzip2TarFixture() }},
		{name: "tbz", file: "a.tbz", build: func(_ *testing.T) []byte { return bzip2TarFixture() }},
		{name: "tbz2", file: "a.tbz2", build: func(_ *testing.T) []byte { return bzip2TarFixture() }},
		{
			name: "tar.zst", file: "a.tar.zst",
			build: func(t *testing.T) []byte { return zstdBytes(t, unpacktest.HelloTar(t)) },
		},
		{name: "tar", file: "a.tar", build: unpacktest.HelloTar},
		{name: "zip", file: "a.zip", build: helloZip},
		{
			name: "upper case", file: "A.TAR.GZ",
			build: func(t *testing.T) []byte { return unpacktest.GzipBytes(t, unpacktest.HelloTar(t)) },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := unpacktest.WriteArchive(t, dir, tc.file, tc.build(t))
			to := filepath.Join(dir, "out")

			require.NoError(t, runUnpack(t, unpacktest.TestMaxBytes, from, to, 0))

			data, err := os.ReadFile(filepath.Join(to, "hello.txt"))
			require.NoError(t, err)
			assert.Contains(t, string(data), "hello")
		})
	}
}

func TestDetectArchive_FallsBackToMagicBytes(t *testing.T) {
	testCases := []struct {
		name     string
		build    func(t *testing.T) []byte
		wantFile string
		wantBody string
	}{
		{name: "tar", build: unpacktest.HelloTar, wantFile: "hello.txt", wantBody: "hello\n"},
		{name: "zip", build: helloZip, wantFile: "hello.txt", wantBody: "hello\n"},
		{
			name:     "tar in gzip",
			build:    func(t *testing.T) []byte { return unpacktest.GzipBytes(t, unpacktest.HelloTar(t)) },
			wantFile: "hello.txt",
			wantBody: "hello\n",
		},
		{
			name:     "tar in xz",
			build:    func(t *testing.T) []byte { return xzBytes(t, unpacktest.HelloTar(t)) },
			wantFile: "hello.txt",
			wantBody: "hello\n",
		},
		{
			name:     "tar in zstd",
			build:    func(t *testing.T) []byte { return zstdBytes(t, unpacktest.HelloTar(t)) },
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
			build:    func(t *testing.T) []byte { return unpacktest.GzipBytes(t, []byte("tool")) },
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
			from := unpacktest.WriteArchive(t, dir, "payload.bin", tc.build(t))
			to := filepath.Join(dir, "out")

			require.NoError(t, runUnpack(t, unpacktest.TestMaxBytes, from, to, 0))

			assert.Equal(t, tc.wantBody, unpacktest.ReadString(t, filepath.Join(to, tc.wantFile)))
		})
	}
}

func TestDetectArchive_UnknownFormat(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
	}{
		{name: "plain text", data: []byte("definitely not an archive")},
		{name: "empty", data: nil},
		{name: "dmg trailer, no longer accepted", data: dmgTrailerFixture()},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := unpacktest.WriteArchive(t, dir, "payload.bin", tc.data)

			err := runUnpack(t, unpacktest.TestMaxBytes, from, filepath.Join(dir, "out"), 0)

			assert.ErrorIs(t, err, unpack.ErrUnknownFormat)
		})
	}
}

func TestIsDmg_Detection(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
		want bool
	}{
		{name: "koly trailer", data: dmgTrailerFixture(), want: true},
		{name: "no trailer", data: make([]byte, 1024), want: false},
		{name: "too short", data: []byte("short"), want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from := unpacktest.WriteArchive(t, dir, "image.bin", tc.data)
			src, err := os.Open(from)
			require.NoError(t, err)
			defer src.Close()
			info, err := src.Stat()
			require.NoError(t, err)

			assert.Equal(t, tc.want, unpack.IsDmg(src, info.Size()))
		})
	}
}

func TestDetectArchive_CorruptCompressedMagic(t *testing.T) {
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
			from := unpacktest.WriteArchive(t, dir, "payload.bin", tc.data)

			err := runUnpack(t, unpacktest.TestMaxBytes, from, filepath.Join(dir, "out"), 0)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "unpack:")
		})
	}
}
