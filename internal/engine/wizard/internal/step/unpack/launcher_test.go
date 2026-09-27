package unpack_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/unpack/unpacktest"
)

func TestWriteLauncher_Content(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "vendor arguments",
			args: []string{"--no-sandbox", "--flag=a b"},
			want: "#!/bin/sh\n" +
				"APPDIR=$(dirname \"$(readlink -f \"$0\")\")\n" +
				"export APPDIR\n" +
				"exec \"$APPDIR/AppRun\" '--no-sandbox' '--flag=a b' \"$@\"\n",
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

			path, err := unpack.WriteLauncher(dir, tc.args)

			require.NoError(t, err)
			assert.Equal(t, filepath.Join(dir, unpack.LauncherName), path)
			assert.True(t, filepath.IsAbs(path))
			assert.Equal(t, tc.want, unpacktest.ReadString(t, path))
			if runtime.GOOS == "windows" {
				return
			}
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
		})
	}
}

func TestWriteLauncher_QuotesSingleQuote(t *testing.T) {
	dir := t.TempDir()

	path, err := unpack.WriteLauncher(dir, []string{"it's", "$HOME"})

	require.NoError(t, err)
	assert.Contains(t, unpacktest.ReadString(t, path), `exec "$APPDIR/AppRun" 'it'\''s' '$HOME' "$@"`)
}

func TestWriteLauncher_RelativePathIsMadeAbsolute(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.Mkdir("app", 0o755))

	path, err := unpack.WriteLauncher("app", nil)

	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(path))
	assert.Equal(t, unpack.LauncherName, filepath.Base(path))
}

func TestWriteLauncher_ReplacesExistingLauncher(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AppRun"), []byte("vendor"), 0o755))
	require.NoError(t, os.Symlink("AppRun", filepath.Join(dir, unpack.LauncherName)))

	path, err := unpack.WriteLauncher(dir, nil)

	require.NoError(t, err)
	info, err := os.Lstat(path)
	require.NoError(t, err)
	assert.True(t, info.Mode().IsRegular())
	assert.Equal(t, "vendor", unpacktest.ReadString(t, filepath.Join(dir, "AppRun")))
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
				require.NoError(t, os.MkdirAll(filepath.Join(dir, unpack.LauncherName, "x"), 0o755))
				return dir
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := unpack.WriteLauncher(tc.setup(t), nil)

			require.Error(t, err)
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
	_, err := unpack.WriteLauncher(appDir, []string{"--no-sandbox", "it's"})
	require.NoError(t, err)

	moved := filepath.Join(base, "renamed", "App.AppDir")
	require.NoError(t, os.MkdirAll(filepath.Dir(moved), 0o755))
	require.NoError(t, os.Rename(appDir, moved))
	binDir := filepath.Join(base, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	link := filepath.Join(binDir, "app")
	require.NoError(t, os.Symlink(filepath.Join(moved, unpack.LauncherName), link))
	out := filepath.Join(base, "out.txt")

	cmd := exec.Command(link, "caller arg")
	cmd.Env = append(os.Environ(), "QUIVER_TEST_OUT="+out)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))

	resolved, err := filepath.EvalSymlinks(moved)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSuffix(unpacktest.ReadString(t, out), "\n"), "\n")
	assert.Equal(t, []string{resolved, "--no-sandbox", "it's", "caller arg"}, lines)
}
