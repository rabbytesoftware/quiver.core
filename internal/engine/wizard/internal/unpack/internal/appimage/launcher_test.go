package appimage

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/unpack/mocks"
)

func TestWriteLauncher_Content(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "vendor arguments",
			args: []string{"--no-sandbox", "--flag=a b", "it's", "$HOME"},
			want: "#!/bin/sh\n" +
				"APPDIR=$(dirname \"$(readlink -f \"$0\")\")\n" +
				"export APPDIR\n" +
				"exec \"$APPDIR/AppRun\" '--no-sandbox' '--flag=a b' 'it'\\''s' '$HOME' \"$@\"\n",
		},
		{
			name: "no vendor arguments",
			args: nil,
			want: "#!/bin/sh\n" +
				"APPDIR=$(dirname \"$(readlink -f \"$0\")\")\n" +
				"export APPDIR\n" +
				"exec \"$APPDIR/AppRun\" \"$@\"\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()

			path := filepath.Join(dir, LauncherName)

			require.NoError(t, writeLauncher(dir, tc.args))
			assert.Equal(t, tc.want, mocks.ReadString(t, path))
			if runtime.GOOS == "windows" {
				return
			}
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
		})
	}
}

func TestWriteLauncher_ReplacesExistingLauncher(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AppRun"), []byte("vendor"), 0o755))
	require.NoError(t, os.Symlink("AppRun", filepath.Join(dir, LauncherName)))

	require.NoError(t, writeLauncher(dir, nil))

	info, err := os.Lstat(filepath.Join(dir, LauncherName))
	require.NoError(t, err)
	assert.True(t, info.Mode().IsRegular())
	assert.Equal(t, "vendor", mocks.ReadString(t, filepath.Join(dir, "AppRun")))
}

func TestWriteLauncher_Errors(t *testing.T) {
	testCases := []struct {
		name  string
		setup func(t *testing.T) string
	}{
		{
			name:  "missing app dir",
			setup: func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing") },
		},
		{
			name: "launcher path is a non-empty directory",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(dir, LauncherName, "x"), 0o755))
				return dir
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, writeLauncher(tc.setup(t), nil))
		})
	}
}

func TestWriteLauncher_RelativeToItself(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the launcher is a POSIX shell script")
	}
	base := t.TempDir()
	appDir := filepath.Join(base, "installed", "App.AppDir")
	require.NoError(t, os.MkdirAll(appDir, 0o755))
	appRun := "#!/bin/sh\nprintf '%s\\n' \"$APPDIR\" \"$@\" > \"$QUIVER_TEST_OUT\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "AppRun"), []byte(appRun), 0o755))
	require.NoError(t, writeLauncher(appDir, []string{"--no-sandbox", "it's"}))

	moved := filepath.Join(base, "renamed", "App.AppDir")
	require.NoError(t, os.MkdirAll(filepath.Dir(moved), 0o755))
	require.NoError(t, os.Rename(appDir, moved))
	binDir := filepath.Join(base, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	link := filepath.Join(binDir, "app")
	require.NoError(t, os.Symlink(filepath.Join(moved, LauncherName), link))
	out := filepath.Join(base, "out.txt")

	cmd := exec.Command(link, "caller arg")
	cmd.Env = append(os.Environ(), "QUIVER_TEST_OUT="+out)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))

	resolved, err := filepath.EvalSymlinks(moved)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSuffix(mocks.ReadString(t, out), "\n"), "\n")
	assert.Equal(t, []string{resolved, "--no-sandbox", "it's", "caller arg"}, lines)
}
