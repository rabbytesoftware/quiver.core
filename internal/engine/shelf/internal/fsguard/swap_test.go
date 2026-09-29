package fsguard

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/engine/shelf/internal/mocks"
)

func TestSwapFile_ReplacesContent(t *testing.T) {
	loc := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(loc, []byte("old"), 0o600))

	require.NoError(t, SwapFile(loc, []byte("new")))

	data, err := os.ReadFile(loc)
	require.NoError(t, err)
	assert.Equal(t, "new", string(data))
	assert.NoFileExists(t, loc+StagedSuffix)
}

func TestSwapFile_NewFileIsWorldReadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes are not represented on windows")
	}
	loc := filepath.Join(t.TempDir(), "file")

	require.NoError(t, SwapFile(loc, []byte("x")))

	info, err := os.Stat(loc)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o044), info.Mode().Perm()&0o044)
}

func TestSwapFile_Errors(t *testing.T) {
	testCases := []struct {
		name  string
		setup func(t *testing.T) string
		check func(t *testing.T, loc string)
	}{
		{
			name: "staged not removable",
			setup: func(t *testing.T) string {
				loc := filepath.Join(t.TempDir(), "file")
				mocks.WriteFile(t, filepath.Join(loc+StagedSuffix, "x"), "", 0o600)
				return loc
			},
			check: func(t *testing.T, loc string) {
				assert.FileExists(t, filepath.Join(loc+StagedSuffix, "x"))
				assert.NoFileExists(t, loc)
			},
		},
		{
			name: "parent missing",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing", "file")
			},
			check: func(t *testing.T, loc string) {
				assert.NoFileExists(t, loc+StagedSuffix)
				assert.NoFileExists(t, loc)
			},
		},
		{
			name: "location is a directory",
			setup: func(t *testing.T) string {
				loc := filepath.Join(t.TempDir(), "file")
				mocks.WriteFile(t, filepath.Join(loc, "x"), "", 0o600)
				return loc
			},
			check: func(t *testing.T, loc string) {
				assert.FileExists(t, loc+StagedSuffix)
				assert.DirExists(t, loc)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			loc := tc.setup(t)

			err := SwapFile(loc, []byte("x"))

			require.Error(t, err)
			tc.check(t, loc)
		})
	}
}

func TestSwapSymlink_Repoints(t *testing.T) {
	mocks.RequireUnixHost(t)
	dir := t.TempDir()
	loc := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(filepath.Join(dir, "old"), loc))

	require.NoError(t, SwapSymlink(filepath.Join(dir, "new"), loc))

	got, err := os.Readlink(loc)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "new"), got)
}

func TestSwapSymlink_Errors(t *testing.T) {
	mocks.RequireUnixHost(t)

	testCases := []struct {
		name  string
		setup func(t *testing.T) string
		check func(t *testing.T, loc string)
	}{
		{
			name: "staged not removable",
			setup: func(t *testing.T) string {
				loc := filepath.Join(t.TempDir(), "link")
				mocks.WriteFile(t, filepath.Join(loc+StagedSuffix, "x"), "", 0o600)
				return loc
			},
			check: func(t *testing.T, loc string) {
				assert.FileExists(t, filepath.Join(loc+StagedSuffix, "x"))
				_, err := os.Lstat(loc)
				assert.True(t, errors.Is(err, fs.ErrNotExist))
			},
		},
		{
			name: "parent missing",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing", "link")
			},
			check: func(t *testing.T, loc string) {
				_, err := os.Lstat(loc + StagedSuffix)
				assert.True(t, errors.Is(err, fs.ErrNotExist))
				_, err = os.Lstat(loc)
				assert.True(t, errors.Is(err, fs.ErrNotExist))
			},
		},
		{
			name: "location is a directory",
			setup: func(t *testing.T) string {
				loc := filepath.Join(t.TempDir(), "link")
				mocks.WriteFile(t, filepath.Join(loc, "x"), "", 0o600)
				return loc
			},
			check: func(t *testing.T, loc string) {
				info, err := os.Lstat(loc + StagedSuffix)
				require.NoError(t, err)
				assert.NotZero(t, info.Mode()&os.ModeSymlink)
				assert.DirExists(t, loc)
				assert.FileExists(t, filepath.Join(loc, "x"))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			loc := tc.setup(t)

			err := SwapSymlink("/target", loc)

			require.Error(t, err)
			tc.check(t, loc)
		})
	}
}
