package portable

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveSource_Decisions(t *testing.T) {
	testCases := []struct {
		name       string
		inWorkDir  bool
		isOutput   bool
		wantExists bool
	}{
		{name: "inside the workdir is removed", inWorkDir: true, wantExists: false},
		{name: "outside the workdir is kept", inWorkDir: false, wantExists: true},
		{name: "the output itself is kept", inWorkDir: true, isOutput: true, wantExists: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			workDir := t.TempDir()
			dir := t.TempDir()
			if tc.inWorkDir {
				dir = workDir
			}
			from := filepath.Join(dir, "src")
			require.NoError(t, os.WriteFile(from, []byte("x"), 0o600))
			output := ""
			if tc.isOutput {
				output = from
			}

			require.NoError(t, removeSource(workDir, from, output))

			_, err := os.Stat(from)
			assert.Equal(t, tc.wantExists, err == nil)
		})
	}
}

func TestRemoveSource_OutsideSymlinkToInsideFileKept(t *testing.T) {
	workDir := t.TempDir()
	target := filepath.Join(workDir, "tool")
	require.NoError(t, os.WriteFile(target, []byte("x"), 0o600))
	link := filepath.Join(t.TempDir(), "tool")
	require.NoError(t, os.Symlink(target, link))

	require.NoError(t, removeSource(workDir, link, ""))

	_, err := os.Lstat(link)
	require.NoError(t, err)
	assert.FileExists(t, target)
}

func TestRemoveSource_FailureIsReturned(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions do not block removal here")
	}

	workDir := t.TempDir()
	locked := filepath.Join(workDir, "locked")
	require.NoError(t, os.Mkdir(locked, 0o755))
	from := filepath.Join(locked, "src")
	require.NoError(t, os.WriteFile(from, []byte("x"), 0o600))
	require.NoError(t, os.Chmod(locked, 0o555))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	err := removeSource(workDir, from, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "portable: remove source")
	assert.FileExists(t, from)
}
