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
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/unpacktest"
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

func TestUnpack_ExtractsDetectedArchive(t *testing.T) {
	dir := t.TempDir()
	from := unpacktest.WriteArchive(t, dir, "a.tar.gz", unpacktest.GzipBytes(t, unpacktest.TarBytes(t,
		unpacktest.TarEntry{Name: "bin/tool", Body: "tool", Mode: 0o755, Flag: tar.TypeReg},
	)))
	src, size := openFile(t, from)

	kind, err := unpack.DetectArchive(src, size)
	require.NoError(t, err)
	g, err := unpack.OpenGuard(context.Background(), filepath.Join(dir, "out"), unpacktest.TestMaxBytes)
	require.NoError(t, err)
	defer g.Close()
	require.NoError(t, kind.Extract(context.Background(), src, size, g))

	assert.Equal(t, "tool", unpacktest.ReadString(t, filepath.Join(dir, "out", "bin", "tool")))
	assert.Equal(t, []string{"bin"}, g.TopLevel())
	assert.False(t, unpack.IsAppImage(src))
	assert.False(t, unpack.IsDmg(src, size))
}

func TestUnpack_ExposesSentinelsFromEveryStage(t *testing.T) {
	dir := t.TempDir()
	unknown := unpacktest.WriteArchive(t, dir, "blob.bin", []byte("not an archive"))
	src, size := openFile(t, unknown)

	_, err := unpack.DetectArchive(src, size)
	assert.ErrorIs(t, err, unpack.ErrUnknownFormat)

	escape := unpacktest.WriteArchive(t, dir, "escape.tar", unpacktest.TarBytes(t,
		unpacktest.TarEntry{Name: "../x", Body: "x", Mode: 0o644, Flag: tar.TypeReg},
	))
	src, size = openFile(t, escape)
	kind, err := unpack.DetectArchive(src, size)
	require.NoError(t, err)
	g, err := unpack.OpenGuard(context.Background(), filepath.Join(dir, "out"), unpacktest.TestMaxBytes)
	require.NoError(t, err)
	defer g.Close()
	assert.ErrorIs(t, kind.Extract(context.Background(), src, size, g), unpack.ErrEscape)
}

func TestUnpack_SkipEscapingLinksOption(t *testing.T) {
	dir := t.TempDir()
	from := unpacktest.WriteArchive(t, dir, "a.tar", unpacktest.TarBytes(t,
		unpacktest.TarEntry{Name: "esc", Flag: tar.TypeSymlink, Link: "../../x", Mode: 0o777},
	))
	src, size := openFile(t, from)
	kind, err := unpack.DetectArchive(src, size)
	require.NoError(t, err)
	g, err := unpack.OpenGuard(context.Background(), filepath.Join(dir, "out"), unpacktest.TestMaxBytes, unpack.SkipEscapingLinks())
	require.NoError(t, err)
	defer g.Close()

	require.NoError(t, kind.Extract(context.Background(), src, size, g))
	require.NoError(t, g.Verify())
}

func TestUnpack_AppImageLifecycle(t *testing.T) {
	entries := map[string]unpacktest.Entry{
		"AppRun":        {Mode: 0o755, Data: "#!/bin/sh\n"},
		"bruno.desktop": {Mode: 0o644, Data: "[Desktop Entry]\nName=Bruno\nExec=AppRun --no-sandbox %U\nIcon=bruno\n"},
		"bruno.png":     {Mode: 0o644, Data: "png"},
	}
	built := unpacktest.BuildAppImage(t, entries, 2)
	src, size := openFile(t, built)
	require.True(t, unpack.IsAppImage(src))
	dir := t.TempDir()
	appDir := filepath.Join(dir, unpack.AppDirName("bruno.AppImage"))

	g, err := unpack.OpenGuard(context.Background(), appDir, unpacktest.TestMaxBytes)
	require.NoError(t, err)
	defer g.Close()
	require.NoError(t, unpack.ExtractAppImage(context.Background(), src, size, g))
	meta, err := unpack.ReadAppImageMeta(appDir)
	require.NoError(t, err)
	launcher, err := unpack.WriteLauncher(appDir, meta.Args)
	require.NoError(t, err)

	assert.Equal(t, "Bruno", meta.Name)
	assert.Equal(t, []string{"--no-sandbox"}, meta.Args)
	assert.Equal(t, "bruno.png", meta.Icon)
	assert.Equal(t, filepath.Join(appDir, unpack.LauncherName), launcher)
}

func TestUnpack_ExtractDmgFailsWithoutImage(t *testing.T) {
	g, err := unpack.OpenGuard(context.Background(), filepath.Join(t.TempDir(), "out"), unpacktest.TestMaxBytes)
	require.NoError(t, err)
	defer g.Close()

	err = unpack.ExtractDmg(context.Background(), "missing.dmg", g)

	assert.Error(t, err)
}

func TestUnpack_SentinelsAreDistinct(t *testing.T) {
	sentinels := []error{
		unpack.ErrEscape, unpack.ErrTooLarge, unpack.ErrTooMany,
		unpack.ErrUnknownFormat, unpack.ErrUnsupportedAppImage, unpack.ErrNoSquashfs,
	}

	for i, a := range sentinels {
		for j, b := range sentinels {
			assert.Equal(t, i == j, errors.Is(a, b))
		}
	}
}
