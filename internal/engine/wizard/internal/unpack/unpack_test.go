package unpack_test

import (
	"archive/tar"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

func openFile(
	t *testing.T,
	path string,
) (*os.File, int64) {
	t.Helper()

	src, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })
	info, err := src.Stat()
	require.NoError(t, err)

	return src, info.Size()
}

func appImageBytes(
	t *testing.T,
) []byte {
	t.Helper()

	data, err := os.ReadFile(mocks.BuildAppImage(t, mocks.AppImageEntries("bruno"), 2))
	require.NoError(t, err)

	return data
}

func TestUnpacker_Detect_OrderedFormats(t *testing.T) {
	ole := append([]byte("\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"), make([]byte, 504)...)

	testCases := []struct {
		name string
		file string
		data func(t *testing.T) []byte
		want unpack.Kind
	}{
		{name: "appimage", file: "bruno.AppImage", data: appImageBytes, want: unpack.KindAppImage},
		{name: "appimage beats an archive suffix", file: "bruno.zip", data: appImageBytes, want: unpack.KindAppImage},
		{name: "dmg", file: "Foo.dmg", data: func(*testing.T) []byte { return mocks.DmgTrailer() }, want: unpack.KindDmg},
		{name: "msi", file: ".tool.download.msi", data: func(*testing.T) []byte { return ole }, want: unpack.KindMsi},
		{name: "archive", file: "a.tar.gz", data: func(t *testing.T) []byte { return mocks.GzipBytes(t, mocks.HelloTar(t)) }, want: unpack.KindArchive},
		{name: "archive by magic", file: ".tool.download", data: mocks.HelloTar, want: unpack.KindArchive},
		{name: "archive suffix beats executable magic", file: "tool.tar", data: func(*testing.T) []byte { return []byte(mocks.ElfExecutable) }, want: unpack.KindArchive},
		{name: "executable", file: "tool", data: func(*testing.T) []byte { return []byte(mocks.ElfExecutable) }, want: unpack.KindBinary},
		{name: "pe executable", file: "setup.exe", data: func(*testing.T) []byte { return []byte("MZ" + "payload") }, want: unpack.KindBinary},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			src, size := openFile(t, mocks.WriteFile(t, filepath.Join(t.TempDir(), tc.file), tc.data(t)))

			format, err := unpack.New(mocks.TestMaxBytes).Detect(src, size)

			require.NoError(t, err)
			assert.Equal(t, tc.want, format.Kind())
		})
	}
}

func TestUnpacker_Detect_Failures(t *testing.T) {
	testCases := []struct {
		name        string
		data        []byte
		wantUnknown bool
	}{
		{name: "plain text", data: []byte("definitely not a package"), wantUnknown: true},
		{name: "empty", data: nil, wantUnknown: true},
		{name: "corrupt gzip", data: []byte{0x1f, 0x8b, 0x00}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			src, size := openFile(t, mocks.WriteFile(t, filepath.Join(t.TempDir(), "payload"), tc.data))

			format, err := unpack.New(mocks.TestMaxBytes).Detect(src, size)

			require.Error(t, err)
			assert.Nil(t, format)
			assert.Equal(t, tc.wantUnknown, errors.Is(err, unpack.ErrUnknownFormat))
		})
	}
}

func TestFormat_UnpacksIntoTheTarget(t *testing.T) {
	dir := t.TempDir()
	src, size := openFile(t, mocks.WriteFile(t, filepath.Join(dir, "a.tar"), mocks.TarBytes(t,
		mocks.TarEntry{Name: "Foo.app/Contents/MacOS/foo", Body: "bin", Mode: 0o755, Flag: tar.TypeReg},
		mocks.TarEntry{Name: "../escape", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
	)))
	format, err := unpack.New(mocks.TestMaxBytes).Detect(src, size)
	require.NoError(t, err)

	_, err = format.Unpack(context.Background(), unpack.Target{Dir: filepath.Join(dir, "out")})

	require.ErrorIs(t, err, unpack.ErrEscape)
	assert.Equal(t, "bin", mocks.ReadString(t, filepath.Join(dir, "out", "Foo.app", "Contents", "MacOS", "foo")))
}
