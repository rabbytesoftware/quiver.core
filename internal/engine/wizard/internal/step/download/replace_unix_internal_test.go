//go:build darwin || linux

package download

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// A leftover that is a symlink (the destination was one) is removed as a
// link: clearing read-only must never follow it and change its target.
func TestSweepStale_SymlinkLeftover_LeavesItsTargetAlone(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "elsewhere")
	require.NoError(t, os.WriteFile(target, nil, 0o400))
	aside := filepath.Join(dir, "tool"+asideMarker+"old")
	require.NoError(t, os.Symlink(target, aside))
	old := time.Now().Add(-2 * staleAge)
	require.NoError(t, unix.Lutimes(aside, []unix.Timeval{unix.NsecToTimeval(old.UnixNano()), unix.NsecToTimeval(old.UnixNano())}))

	sweepStale(filepath.Join(dir, "tool"))

	_, err := os.Lstat(aside)
	assert.ErrorIs(t, err, os.ErrNotExist, "the stale link is removed")
	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o400), info.Mode().Perm(), "its target keeps its mode")
}
