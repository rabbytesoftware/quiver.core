package appimage_test

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/appimage"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/guard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

func extractAppImage(
	t *testing.T,
	ctx context.Context,
	path string,
	to string,
	maxBytes int64,
) error {
	t.Helper()

	src, err := os.Open(path)
	require.NoError(t, err)
	defer src.Close()
	info, err := src.Stat()
	require.NoError(t, err)

	g, err := guard.Open(context.Background(), to, maxBytes, guard.SkipEscapingLinks())
	require.NoError(t, err)
	defer g.Close()

	if err := appimage.Extract(ctx, src, info.Size(), g); err != nil {
		return err
	}

	return g.Verify()
}

func TestAppDirName_StripsAppImageSuffix(t *testing.T) {
	testCases := []struct {
		name string
		from string
		want string
	}{
		{name: "canonical suffix", from: "/tmp/in/bruno_4.2.0_arm64_linux.AppImage", want: "bruno_4.2.0_arm64_linux"},
		{name: "lowercase suffix", from: "app.appimage", want: "app"},
		{name: "no suffix", from: "/tmp/in/tool", want: "tool.AppDir"},
		{name: "suffix only", from: ".AppImage", want: ".AppImage.AppDir"},
		{name: "dot stem", from: "/tmp/in/..AppImage", want: "..AppImage.AppDir"},
		{name: "dot-dot stem", from: "/tmp/in/...AppImage", want: "...AppImage.AppDir"},
		{name: "dot-dot base", from: "/tmp/in/..", want: "...AppDir"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, appimage.DirName(tc.from))
		})
	}
}

func TestExtractAppImage_WritesTree(t *testing.T) {
	path := mocks.BuildAppImage(t, map[string]mocks.Entry{
		"AppRun":                   {Mode: 0o755, Data: "#!/bin/sh\necho hi\n"},
		"app.desktop":              {Mode: 0o644, Data: "[Desktop Entry]\nName=App\n"},
		"usr/lib/libx.so.1":        {Mode: 0o644, Data: "lib"},
		"usr/lib/libx.so":          {Link: "libx.so.1"},
		"usr/share/empty":          {Mode: fs.ModeDir | 0o755},
		"usr/share/icons/app.png":  {Mode: 0o644, Data: "png"},
		".DirIcon":                 {Link: "usr/share/icons/app.png"},
		"usr/share/doc/README.txt": {Mode: 0o644, Data: "readme"},
	}, 2)
	to := filepath.Join(t.TempDir(), "app")

	require.NoError(t, extractAppImage(t, context.Background(), path, to, mocks.TestMaxBytes))

	assert.Equal(t, "#!/bin/sh\necho hi\n", mocks.ReadString(t, filepath.Join(to, "AppRun")))
	assert.Equal(t, "lib", mocks.ReadString(t, filepath.Join(to, "usr", "lib", "libx.so")))
	assert.Equal(t, "readme", mocks.ReadString(t, filepath.Join(to, "usr", "share", "doc", "README.txt")))
	assert.DirExists(t, filepath.Join(to, "usr", "share", "empty"))

	target, err := os.Readlink(filepath.Join(to, ".DirIcon"))
	require.NoError(t, err)
	assert.Equal(t, "usr/share/icons/app.png", target)

	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(filepath.Join(to, "AppRun"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	info, err = os.Stat(filepath.Join(to, "app.desktop"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
}

func TestExtractAppImage_EscapingSymlinkSkipped(t *testing.T) {
	path := mocks.BuildAppImage(t, map[string]mocks.Entry{
		"AppRun":   {Mode: 0o755, Data: "run"},
		".DirIcon": {Link: "/home/runner/x.png"},
		"a":        {Link: "../../etc"},
		"usr/up":   {Link: "../.."},
		"b":        {Link: "AppRun"},
	}, 2)
	to := filepath.Join(t.TempDir(), "app")

	require.NoError(t, extractAppImage(t, context.Background(), path, to, mocks.TestMaxBytes))

	for _, name := range []string{".DirIcon", "a", "usr/up"} {
		_, err := os.Lstat(filepath.Join(to, filepath.FromSlash(name)))
		assert.ErrorIs(t, err, os.ErrNotExist, name)
	}
	assert.Equal(t, "run", mocks.ReadString(t, filepath.Join(to, "b")))
}

func TestExtractAppImage_Type1Unsupported(t *testing.T) {
	path := mocks.BuildAppImage(t, map[string]mocks.Entry{"AppRun": {Data: "x"}}, 1)

	err := extractAppImage(t, context.Background(), path, filepath.Join(t.TempDir(), "app"), mocks.TestMaxBytes)

	require.ErrorIs(t, err, models.ErrUnsupportedAppImage)
}

func TestExtractAppImage_NoSquashfs(t *testing.T) {
	testCases := []struct {
		name string
		tail []byte
	}{
		{name: "garbage after the runtime", tail: []byte("this is definitely not a squashfs image")},
		{name: "nothing after the runtime", tail: nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := mocks.WriteFile(t, filepath.Join(t.TempDir(), "x.AppImage"), append(mocks.ElfHeader(2), tc.tail...))

			err := extractAppImage(t, context.Background(), path, filepath.Join(t.TempDir(), "app"), mocks.TestMaxBytes)

			require.ErrorIs(t, err, models.ErrNoSquashfs)
		})
	}
}

func TestExtractAppImage_CorruptSquashfs(t *testing.T) {
	to := filepath.Join(t.TempDir(), "app")
	path := mocks.WriteFile(t, filepath.Join(t.TempDir(), "x.AppImage"), append(mocks.ElfHeader(2), []byte("hsqs-but-not-really")...))

	err := extractAppImage(t, context.Background(), path, to, mocks.TestMaxBytes)

	require.Error(t, err)
	assert.NotErrorIs(t, err, models.ErrNoSquashfs)
	assertNoAppDirContents(t, to)
}

func TestExtractAppImage_NotElf(t *testing.T) {
	to := filepath.Join(t.TempDir(), "app")
	data := []byte("\x00\x00\x00\x00\x00\x00\x00\x00AI\x02 and some more padding bytes here")
	path := mocks.WriteFile(t, filepath.Join(t.TempDir(), "x.AppImage"), data)

	err := extractAppImage(t, context.Background(), path, to, mocks.TestMaxBytes)

	require.Error(t, err)
	assertNoAppDirContents(t, to)
}

func assertNoAppDirContents(
	t *testing.T,
	to string,
) {
	t.Helper()

	entries, err := os.ReadDir(to)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestExtractAppImage_ShortFile(t *testing.T) {
	path := mocks.WriteFile(t, filepath.Join(t.TempDir(), "x.AppImage"), []byte("\x7fELF"))

	err := extractAppImage(t, context.Background(), path, filepath.Join(t.TempDir(), "app"), mocks.TestMaxBytes)

	require.Error(t, err)
	assert.NotErrorIs(t, err, models.ErrUnsupportedAppImage)
}

func TestExtractAppImage_TooLarge(t *testing.T) {
	path := mocks.BuildAppImage(t, map[string]mocks.Entry{
		"AppRun": {Mode: 0o755, Data: "this body is longer than ten bytes"},
	}, 2)

	err := extractAppImage(t, context.Background(), path, filepath.Join(t.TempDir(), "app"), 10)

	require.ErrorIs(t, err, models.ErrTooLarge)
}

func TestExtractAppImage_ContextCanceled(t *testing.T) {
	testCases := []struct {
		name    string
		entries map[string]mocks.Entry
	}{
		{name: "flat tree", entries: map[string]mocks.Entry{"AppRun": {Mode: 0o755, Data: "x"}}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := mocks.BuildAppImage(t, tc.entries, 2)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			err := extractAppImage(t, ctx, path, filepath.Join(t.TempDir(), "app"), mocks.TestMaxBytes)

			require.ErrorIs(t, err, context.Canceled)
		})
	}
}

func TestExtractAppImage_EntryErrorsPropagate(t *testing.T) {
	path := mocks.BuildAppImage(t, map[string]mocks.Entry{
		"usr/bin/app": {Mode: 0o755, Data: "x"},
	}, 2)
	to := filepath.Join(t.TempDir(), "app")
	require.NoError(t, os.MkdirAll(to, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(to, "usr"), []byte("file in the way"), 0o600))

	err := extractAppImage(t, context.Background(), path, to, mocks.TestMaxBytes)

	require.Error(t, err)
}

func TestIsAppImage_Detection(t *testing.T) {
	testCases := []struct {
		name string
		head []byte
		want bool
	}{
		{"type2", append([]byte("\x7fELF\x02\x01\x01\x00"), []byte("AI\x02")...), true},
		{"type1", append([]byte("\x7fELF\x02\x01\x01\x00"), []byte("AI\x01")...), true},
		{"plain elf", []byte("\x7fELF\x02\x01\x01\x00\x00\x00\x00"), false},
		{"zip", []byte("PK\x03\x04\x00\x00\x00\x00\x00\x00\x00"), false},
		{"short", []byte("\x7fELF"), false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := appimage.Is(bytes.NewReader(tc.head))

			assert.Equal(t, tc.want, got)
		})
	}
}
