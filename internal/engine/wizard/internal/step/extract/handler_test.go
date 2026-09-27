package extract_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/ulikunitz/xz"

	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	stepextract "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/extract"
)

const testMaxBytes = int64(1 << 20)

type tarEntry struct {
	name string
	body string
	mode int64
	flag byte
	link string
}

type zipEntry struct {
	name string
	body string
	mode os.FileMode
}

func helloTar(
	t *testing.T,
) []byte {
	t.Helper()
	return tarBytes(t, tarEntry{name: "hello.txt", body: "hello\n", mode: 0o644, flag: tar.TypeReg})
}

func helloZip(
	t *testing.T,
) []byte {
	t.Helper()
	return zipBytes(t, zipEntry{name: "hello.txt", body: "hello\n", mode: 0o644})
}

func tarBytes(
	t *testing.T,
	entries ...tarEntry,
) []byte {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     e.mode,
			Typeflag: e.flag,
			Linkname: e.link,
			Size:     int64(len(e.body)),
			Format:   tar.FormatPAX,
		}
		require.NoError(t, tw.WriteHeader(hdr))
		_, err := tw.Write([]byte(e.body))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())

	return buf.Bytes()
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

func gzipBytes(
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

func writeArchive(
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

func runExtract(
	t *testing.T,
	maxBytes int64,
	from string,
	to string,
	timeout string,
) error {
	t.Helper()

	h := stepextract.NewHandler(maxBytes)
	s := domainstep.NewExtractStep("extract", from, to, timeout, true)

	return h.Execute(context.Background(), wizstep.Request{WorkDir: filepath.Dir(from)}, s)
}

func readString(
	t *testing.T,
	path string,
) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(data)
}

func TestHandler_Execute_RelativePathsJoinWorkDir(t *testing.T) {
	workDir := t.TempDir()
	writeArchive(t, workDir, "a.tar.gz", gzipBytes(t, helloTar(t)))

	h := stepextract.NewHandler(testMaxBytes)
	s := domainstep.NewExtractStep("extract", "a.tar.gz", "out", "", true)
	err := h.Execute(context.Background(), wizstep.Request{WorkDir: workDir}, s)

	require.NoError(t, err)
	assert.Equal(t, "hello\n", readString(t, filepath.Join(workDir, "out", "hello.txt")))
}

func TestHandler_Execute_ExpandsVariables(t *testing.T) {
	workDir := t.TempDir()
	installDir := t.TempDir()
	writeArchive(t, installDir, "a.tgz", gzipBytes(t, helloTar(t)))

	h := stepextract.NewHandler(testMaxBytes)
	s := domainstep.NewExtractStep("extract", "${INSTALL_PATH}/a.tgz", "${INSTALL_PATH}/${DIR}", "", true)
	req := wizstep.Request{WorkDir: workDir, Vars: map[string]string{"INSTALL_PATH": installDir, "DIR": "bin"}}
	err := h.Execute(context.Background(), req, s)

	require.NoError(t, err)
	assert.Equal(t, "hello\n", readString(t, filepath.Join(installDir, "bin", "hello.txt")))
}

func TestHandler_Execute_ResolvesPlatformOverrides(t *testing.T) {
	dir := t.TempDir()
	writeArchive(t, dir, "a.tar", helloTar(t))

	h := stepextract.NewHandler(testMaxBytes)
	s := domainstep.NewExtractStep("extract", "missing.tar", "wrong", "", true)
	s.From.OSArch = map[string]string{"linux/amd64": "a.tar"}
	s.To.OSArch = map[string]string{"linux/amd64": "right"}
	err := h.Execute(context.Background(), wizstep.Request{WorkDir: dir, OSArch: "linux/amd64"}, s)

	require.NoError(t, err)
	assert.Equal(t, "hello\n", readString(t, filepath.Join(dir, "right", "hello.txt")))
}

func TestHandler_Execute_Failures(t *testing.T) {
	testCases := []struct {
		name    string
		setup   func(t *testing.T, dir string) (string, string)
		timeout string
		wantErr error
		wantMsg string
	}{
		{
			name: "invalid timeout",
			setup: func(t *testing.T, dir string) (string, string) {
				return writeArchive(t, dir, "a.tar", helloTar(t)), filepath.Join(dir, "out")
			},
			timeout: "soon",
			wantMsg: "invalid timeout",
		},
		{
			name: "expired timeout",
			setup: func(t *testing.T, dir string) (string, string) {
				return writeArchive(t, dir, "a.tar", helloTar(t)), filepath.Join(dir, "out")
			},
			timeout: "1ns",
			wantErr: context.DeadlineExceeded,
		},
		{
			name: "missing input",
			setup: func(t *testing.T, dir string) (string, string) {
				return filepath.Join(dir, "absent.tar.gz"), filepath.Join(dir, "out")
			},
			wantErr: os.ErrNotExist,
		},
		{
			name: "input is a directory",
			setup: func(t *testing.T, dir string) (string, string) {
				src := filepath.Join(dir, "srcdir")
				require.NoError(t, os.Mkdir(src, 0o755))
				return src, filepath.Join(dir, "out")
			},
			wantMsg: "extract",
		},
		{
			name: "destination is a file",
			setup: func(t *testing.T, dir string) (string, string) {
				return writeArchive(t, dir, "a.tar", helloTar(t)), writeArchive(t, dir, "blocker", []byte("x"))
			},
			wantMsg: "extract: create",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from, to := tc.setup(t, dir)

			err := runExtract(t, testMaxBytes, from, to, tc.timeout)

			require.Error(t, err)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			}
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

func TestHandler_Execute_UnopenableDestination(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions")
	}

	dir := t.TempDir()
	from := writeArchive(t, dir, "a.tar", helloTar(t))
	to := filepath.Join(dir, "locked")
	require.NoError(t, os.Mkdir(to, 0o755))
	require.NoError(t, os.Chmod(to, 0o000))
	t.Cleanup(func() { _ = os.Chmod(to, 0o755) })

	err := runExtract(t, testMaxBytes, from, to, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "extract: open destination")
}
