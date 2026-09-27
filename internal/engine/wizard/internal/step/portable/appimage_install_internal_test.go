package portable

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack"
)

func TestAppDirPaths_RejectsNamesOutsideDestination(t *testing.T) {
	testCases := []struct {
		name    string
		appName string
	}{
		{name: "empty", appName: ""},
		{name: "dot", appName: "."},
		{name: "dot-dot", appName: ".."},
		{name: "nested", appName: "a/b"},
		{name: "climbs out", appName: "../x"},
		{name: "collapses to destination", appName: "a/.."},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			to := filepath.Join(t.TempDir(), "to")

			appDir, staging, err := appDirPaths(to, tc.appName)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "is not a direct child of")
			assert.Empty(t, appDir)
			assert.Empty(t, staging)
		})
	}
}

func TestAppDirPaths_StagingIsHiddenSibling(t *testing.T) {
	to := t.TempDir()

	appDir, staging, err := appDirPaths(to, "bruno")

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(to, "bruno"), appDir)
	assert.Equal(t, filepath.Join(to, ".bruno.quiver-tmp"), staging)
}

func TestSwapAppDir_MissingStagingFails(t *testing.T) {
	to := t.TempDir()

	err := swapAppDir(filepath.Join(to, ".app.quiver-tmp"), filepath.Join(to, "app"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "portable: move")
	assert.NoDirExists(t, filepath.Join(to, "app"))
}

func TestRemoveOwnedAppDir_Description(t *testing.T) {
	testCases := []struct {
		name       string
		setup      func(t *testing.T, appDir string)
		wantErr    error
		wantExists bool
	}{
		{
			name:  "missing appdir is a no-op",
			setup: func(t *testing.T, appDir string) {},
		},
		{
			name: "owned appdir is removed",
			setup: func(t *testing.T, appDir string) {
				require.NoError(t, os.MkdirAll(filepath.Join(appDir, "usr"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(appDir, unpack.LauncherName), []byte("x"), 0o600))
			},
		},
		{
			name: "directory without a launcher is not owned",
			setup: func(t *testing.T, appDir string) {
				require.NoError(t, os.MkdirAll(appDir, 0o755))
			},
			wantErr:    ErrUnownedAppDir,
			wantExists: true,
		},
		{
			name: "regular file is not owned",
			setup: func(t *testing.T, appDir string) {
				require.NoError(t, os.WriteFile(appDir, []byte("x"), 0o600))
			},
			wantErr:    ErrUnownedAppDir,
			wantExists: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			appDir := filepath.Join(t.TempDir(), "app")
			tc.setup(t, appDir)

			err := removeOwnedAppDir(appDir)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			_, statErr := os.Lstat(appDir)
			assert.Equal(t, tc.wantExists, statErr == nil)
		})
	}
}

func TestRemoveOwnedAppDir_PermissionErrors(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits do not block access here")
	}

	testCases := []struct {
		name    string
		locked  func(to, appDir string) string
		wantMsg string
	}{
		{
			name:    "unsearchable parent",
			locked:  func(to, appDir string) string { return to },
			wantMsg: "portable: stat",
		},
		{
			name:    "undeletable contents",
			locked:  func(to, appDir string) string { return filepath.Join(appDir, "usr") },
			wantMsg: "portable: remove previous",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			to := filepath.Join(t.TempDir(), "to")
			appDir := filepath.Join(to, "app")
			require.NoError(t, os.MkdirAll(filepath.Join(appDir, "usr"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(appDir, "usr", "lib"), []byte("x"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(appDir, unpack.LauncherName), []byte("x"), 0o600))
			locked := tc.locked(to, appDir)
			require.NoError(t, os.Chmod(locked, 0o400))
			t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

			err := removeOwnedAppDir(appDir)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}
