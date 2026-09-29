package install

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack"
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			to := filepath.Join(t.TempDir(), "to")

			appDir, staging, err := appDirPaths(to, tc.appName)

			require.Error(t, err)
			assert.Empty(t, appDir)
			assert.Empty(t, staging)
		})
	}
}

func TestSwapAppDir_MissingStagingFails(t *testing.T) {
	to := t.TempDir()

	err := swapAppDir(filepath.Join(to, ".app.quiver-tmp"), filepath.Join(to, "app"))

	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.NoDirExists(t, filepath.Join(to, "app"))
}

func TestRemoveOwnedAppDir_PermissionErrors(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits do not block access here")
	}

	testCases := []struct {
		name   string
		locked func(to, appDir string) string
	}{
		{
			name:   "unsearchable parent",
			locked: func(to, appDir string) string { return to },
		},
		{
			name:   "undeletable contents",
			locked: func(to, appDir string) string { return filepath.Join(appDir, "usr") },
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
			assert.ErrorIs(t, err, fs.ErrPermission)
		})
	}
}
