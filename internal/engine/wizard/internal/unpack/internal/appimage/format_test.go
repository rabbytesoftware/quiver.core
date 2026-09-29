package appimage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/guard"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/internal/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

func detectFile(
	t *testing.T,
	path string,
) (models.Format, bool) {
	t.Helper()

	src, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })
	info, err := src.Stat()
	require.NoError(t, err)

	format, ok, err := New(mocks.TestMaxBytes, guard.HostRules{FoldCase: true})(src, info.Size())
	require.NoError(t, err)

	return format, ok
}

func TestNew_DetectsAppImagesOnly(t *testing.T) {
	dir := t.TempDir()
	image := mocks.WriteAppImage(t, dir, "bruno.AppImage", mocks.AppImageEntries("bruno"))
	elf := mocks.WriteFile(t, filepath.Join(dir, "tool"), []byte(mocks.ElfExecutable))

	format, ok := detectFile(t, image)
	require.True(t, ok)
	assert.Equal(t, models.KindAppImage, format.Kind())
	assert.Equal(t, models.Unit{Dir: "bruno", Proof: LauncherName}, format.Unit())

	_, ok = detectFile(t, elf)
	assert.False(t, ok)
}

func TestUnpack_WritesAppDirLauncherAndApp(t *testing.T) {
	testCases := []struct {
		name    string
		entries map[string]mocks.Entry
		want    func(appDir string) models.App
	}{
		{
			name:    "desktop entry names the app and its icon",
			entries: mocks.AppImageEntries("bruno"),
			want: func(appDir string) models.App {
				return models.App{
					Name:  "bruno",
					Entry: filepath.Join(appDir, LauncherName),
					Icon:  filepath.Join(appDir, "usr", "share", "icons", "hicolor", "256x256", "apps", "bruno.png"),
				}
			},
		},
		{
			name:    "no desktop entry falls back to the appdir name",
			entries: map[string]mocks.Entry{"AppRun": {Mode: 0o755, Data: "#!/bin/sh\n"}},
			want: func(appDir string) models.App {
				return models.App{Name: "tool", Entry: filepath.Join(appDir, LauncherName)}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			image := mocks.WriteAppImage(t, t.TempDir(), "tool.AppImage", tc.entries)
			format, ok := detectFile(t, image)
			require.True(t, ok)
			appDir := filepath.Join(t.TempDir(), "tool")

			result, err := format.Unpack(context.Background(), models.Target{Dir: appDir})

			require.NoError(t, err)
			assert.Equal(t, []models.App{tc.want(appDir)}, result.Apps)
			assert.Empty(t, result.Output)
			assert.FileExists(t, filepath.Join(appDir, LauncherName))
		})
	}
}

func TestUnpack_Failures(t *testing.T) {
	testCases := []struct {
		name    string
		entries map[string]mocks.Entry
		target  func(t *testing.T) string
	}{
		{
			name:    "appdir under a file",
			entries: mocks.AppImageEntries("bruno"),
			target: func(t *testing.T) string {
				return filepath.Join(mocks.WriteFile(t, filepath.Join(t.TempDir(), "file"), []byte("x")), "app")
			},
		},
		{
			name: "launcher location is a directory",
			entries: map[string]mocks.Entry{
				"AppRun":                  {Mode: 0o755, Data: "#!/bin/sh\n"},
				LauncherName + "/blocker": {Mode: 0o644, Data: "x"},
			},
			target: func(t *testing.T) string { return filepath.Join(t.TempDir(), "app") },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			image := mocks.WriteAppImage(t, t.TempDir(), "bruno.AppImage", tc.entries)
			format, ok := detectFile(t, image)
			require.True(t, ok)

			_, err := format.Unpack(context.Background(), models.Target{Dir: tc.target(t)})

			require.Error(t, err)
		})
	}
}
