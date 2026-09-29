package extract_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	stepextract "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/extract"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

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

func TestHandler_Execute_ExpandsVariables(t *testing.T) {
	workDir := t.TempDir()
	installDir := t.TempDir()
	mocks.WriteFile(t, filepath.Join(installDir, "a.tgz"), mocks.GzipBytes(t, mocks.HelloTar(t)))

	h := stepextract.NewHandler(mocks.TestMaxBytes)
	s := domainstep.NewExtractStep("extract", "${INSTALL_PATH}/a.tgz", "${INSTALL_PATH}/${DIR}", "", true)
	req := wizstep.Request{WorkDir: workDir, Vars: map[string]string{"INSTALL_PATH": installDir, "DIR": "bin"}}
	err := h.Execute(context.Background(), req, s)

	require.NoError(t, err)
	assert.Equal(t, "hello\n", mocks.ReadString(t, filepath.Join(installDir, "bin", "hello.txt")))
}

func TestHandler_Execute_ResolvesPlatformOverrides(t *testing.T) {
	dir := t.TempDir()
	mocks.WriteFile(t, filepath.Join(dir, "a.tar"), mocks.HelloTar(t))

	h := stepextract.NewHandler(mocks.TestMaxBytes)
	s := domainstep.NewExtractStep("extract", "missing.tar", "wrong", "", true)
	s.From.OSArch = map[string]string{"linux/amd64": "a.tar"}
	s.To.OSArch = map[string]string{"linux/amd64": "right"}
	err := h.Execute(context.Background(), wizstep.Request{WorkDir: dir, OSArch: "linux/amd64"}, s)

	require.NoError(t, err)
	assert.Equal(t, "hello\n", mocks.ReadString(t, filepath.Join(dir, "right", "hello.txt")))
}

func TestHandler_Execute_Failures(t *testing.T) {
	testCases := []struct {
		name    string
		setup   func(t *testing.T, dir string) (string, string)
		timeout string
		wantErr error
		check   func(t *testing.T, to string)
	}{
		{
			name: "invalid timeout",
			setup: func(t *testing.T, dir string) (string, string) {
				return mocks.WriteFile(t, filepath.Join(dir, "a.tar"), mocks.HelloTar(t)), filepath.Join(dir, "out")
			},
			timeout: "soon",
			check: func(t *testing.T, to string) {
				assert.NoDirExists(t, to)
			},
		},
		{
			name: "expired timeout",
			setup: func(t *testing.T, dir string) (string, string) {
				return mocks.WriteFile(t, filepath.Join(dir, "a.tar"), mocks.HelloTar(t)), filepath.Join(dir, "out")
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
			check: func(t *testing.T, to string) {
				assert.NoDirExists(t, to)
			},
		},
		{
			name: "destination is a file",
			setup: func(t *testing.T, dir string) (string, string) {
				from := mocks.WriteFile(t, filepath.Join(dir, "a.tar"), mocks.HelloTar(t))
				to := mocks.WriteFile(t, filepath.Join(dir, "blocker"), []byte("x"))
				return from, to
			},
			check: func(t *testing.T, to string) {
				assert.Equal(t, "x", mocks.ReadString(t, to))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			from, to := tc.setup(t, dir)

			err := runExtract(t, mocks.TestMaxBytes, from, to, tc.timeout)

			require.Error(t, err)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			}
			if tc.check != nil {
				tc.check(t, to)
			}
		})
	}
}

func TestHandler_Execute_UnopenableDestination(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions")
	}

	dir := t.TempDir()
	from := mocks.WriteFile(t, filepath.Join(dir, "a.tar"), mocks.HelloTar(t))
	to := filepath.Join(dir, "locked")
	require.NoError(t, os.Mkdir(to, 0o755))
	require.NoError(t, os.Chmod(to, 0o000))
	t.Cleanup(func() { _ = os.Chmod(to, 0o755) })

	err := runExtract(t, mocks.TestMaxBytes, from, to, "")

	require.Error(t, err)
	require.ErrorIs(t, err, fs.ErrPermission)
}

func TestHandler_Execute_DmgTrailerReturnsPortableFormat(t *testing.T) {
	dir := t.TempDir()
	data := make([]byte, 1024)
	copy(data[512:], "koly")
	from := mocks.WriteFile(t, filepath.Join(dir, "image.bin"), data)

	err := runExtract(t, mocks.TestMaxBytes, from, filepath.Join(dir, "out"), "")

	require.ErrorIs(t, err, stepextract.ErrPortableFormat)
}

func TestHandler_Execute_AppImageMagicReturnsPortableFormat(t *testing.T) {
	dir := t.TempDir()
	data := append([]byte("\x7fELF\x02\x01\x01\x00"), []byte("AI\x02")...)
	from := mocks.WriteFile(t, filepath.Join(dir, "app.bin"), data)

	err := runExtract(t, mocks.TestMaxBytes, from, filepath.Join(dir, "out"), "")

	require.ErrorIs(t, err, stepextract.ErrPortableFormat)
}
