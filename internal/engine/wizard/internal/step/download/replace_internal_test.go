package download

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStagingPath_IsBesideTheDestinationAndUnique(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "quiver-new")

	a, err := stagingPath(dst)
	require.NoError(t, err)
	b, err := stagingPath(dst)
	require.NoError(t, err)

	assert.Equal(t, filepath.Dir(dst), filepath.Dir(a))
	assert.True(t, strings.HasPrefix(filepath.Base(a), ".quiver-new"+stagingMarker))
	assert.NotEqual(t, a, b)
}

func TestReplaceFile(t *testing.T) {
	refused := errors.New("in use")

	testCases := []struct {
		name        string
		refuse      func(call int, oldPath, newPath string) bool
		wantErr     bool
		wantContent string
		wantAside   bool
	}{
		{
			name:        "replaced in one rename",
			refuse:      func(int, string, string) bool { return false },
			wantContent: "new",
		},
		{
			name:        "a target in use is moved aside first",
			refuse:      func(call int, _, _ string) bool { return call == 1 },
			wantContent: "new",
			wantAside:   true,
		},
		{
			name:        "a target that cannot be moved aside is kept",
			refuse:      func(call int, _, _ string) bool { return call <= 2 },
			wantErr:     true,
			wantContent: "old",
		},
		{
			name:        "a staged file that still cannot land puts the target back",
			refuse:      func(call int, _, _ string) bool { return call == 1 || call == 3 },
			wantErr:     true,
			wantContent: "old",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			dst := filepath.Join(dir, "quiver-new")
			staged := filepath.Join(dir, ".quiver-new"+stagingMarker+"x")
			require.NoError(t, os.WriteFile(dst, []byte("old"), 0o600))
			require.NoError(t, os.WriteFile(staged, []byte("new"), 0o600))
			calls := 0
			rename := func(oldPath, newPath string) error {
				calls++
				if tc.refuse(calls, oldPath, newPath) {
					return refused
				}
				return os.Rename(oldPath, newPath)
			}

			err := replaceFile(staged, dst, rename)

			if tc.wantErr {
				require.ErrorIs(t, err, refused)
			} else {
				require.NoError(t, err)
			}
			got, readErr := os.ReadFile(dst) // #nosec G304 -- a temp file this test owns
			require.NoError(t, readErr)
			assert.Equal(t, tc.wantContent, string(got))
			aside, _ := filepath.Glob(filepath.Join(dir, "quiver-new"+asideMarker+"*"))
			assert.Equal(t, tc.wantAside, len(aside) == 1, "moved-aside files: %v", aside)
		})
	}
}

func TestSweepStale_RemovesOldLeftoversOfThisDestinationOnly(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "quiver-new")
	old := time.Now().Add(-2 * staleAge)
	for _, name := range []string{
		"quiver-new", ".quiver-new" + stagingMarker + "dead", "quiver-new" + asideMarker + "old",
		"other" + asideMarker + "old", "notes.txt", ".quiver-new" + stagingMarker + "running",
	} {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, nil, 0o600))
		if !strings.HasSuffix(name, "running") {
			require.NoError(t, os.Chtimes(path, old, old))
		}
	}

	sweepStale(dst)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var left []string
	for _, e := range entries {
		left = append(left, e.Name())
	}
	assert.ElementsMatch(t, []string{
		"quiver-new", "other" + asideMarker + "old", "notes.txt", ".quiver-new" + stagingMarker + "running",
	}, left, "a concurrent fetch's fresh staging file is never swept")
}

func TestSweepStale_MissingDirectoryIsANoOp(t *testing.T) {
	assert.NotPanics(t, func() { sweepStale(filepath.Join(t.TempDir(), "gone", "quiver-new")) })
}

func TestStagingPath_DirectoryDestinationIsRefused(t *testing.T) {
	_, err := stagingPath(t.TempDir())

	require.ErrorIs(t, err, ErrDestinationIsDirectory)
}

// Windows refuses to remove a read-only file, and an aside file keeps the
// mode of the destination it was: the sweep clears read-only first.
func TestSweepStale_ReadOnlyLeftoverIsRemoved(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "tool.exe")
	aside := filepath.Join(dir, "tool.exe"+asideMarker+"old")
	require.NoError(t, os.WriteFile(aside, nil, 0o400))
	old := time.Now().Add(-2 * staleAge)
	require.NoError(t, os.Chtimes(aside, old, old))
	refuseReadOnly := func(path string) error {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if info.Mode().Perm()&0o200 == 0 {
			return errors.New("access denied")
		}
		return os.Remove(path)
	}

	sweepStaleWith(dst, refuseReadOnly)

	assert.NoFileExists(t, aside)
}
