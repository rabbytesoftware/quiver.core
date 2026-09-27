package shelf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSwapFile_ReplacesContent(t *testing.T) {
	loc := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(loc, []byte("old"), 0o600))

	require.NoError(t, swapFile(loc, []byte("new")))

	data, err := os.ReadFile(loc)
	require.NoError(t, err)
	assert.Equal(t, "new", string(data))
	assert.NoFileExists(t, loc+stagedSuffix)
}

func TestSwapFile_Errors(t *testing.T) {
	testCases := []struct {
		name  string
		setup func(t *testing.T) string
		want  string
	}{
		{
			name: "staged not removable",
			setup: func(t *testing.T) string {
				loc := filepath.Join(t.TempDir(), "file")
				writeFile(t, filepath.Join(loc+stagedSuffix, "x"), "", 0o600)
				return loc
			},
			want: "clear",
		},
		{
			name: "parent missing",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing", "file")
			},
			want: "write",
		},
		{
			name: "location is a directory",
			setup: func(t *testing.T) string {
				loc := filepath.Join(t.TempDir(), "file")
				writeFile(t, filepath.Join(loc, "x"), "", 0o600)
				return loc
			},
			want: "swap",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := swapFile(tc.setup(t), []byte("x"))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestSwapSymlink_Repoints(t *testing.T) {
	requireUnixHost(t)
	dir := t.TempDir()
	loc := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(filepath.Join(dir, "old"), loc))

	require.NoError(t, swapSymlink(filepath.Join(dir, "new"), loc))

	got, err := os.Readlink(loc)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "new"), got)
}

func TestSwapSymlink_Errors(t *testing.T) {
	requireUnixHost(t)

	testCases := []struct {
		name  string
		setup func(t *testing.T) string
		want  string
	}{
		{
			name: "staged not removable",
			setup: func(t *testing.T) string {
				loc := filepath.Join(t.TempDir(), "link")
				writeFile(t, filepath.Join(loc+stagedSuffix, "x"), "", 0o600)
				return loc
			},
			want: "clear",
		},
		{
			name: "parent missing",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing", "link")
			},
			want: "symlink",
		},
		{
			name: "location is a directory",
			setup: func(t *testing.T) string {
				loc := filepath.Join(t.TempDir(), "link")
				writeFile(t, filepath.Join(loc, "x"), "", 0o600)
				return loc
			},
			want: "swap",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := swapSymlink("/target", tc.setup(t))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
