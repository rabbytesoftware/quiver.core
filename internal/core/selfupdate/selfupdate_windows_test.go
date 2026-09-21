//go:build windows

package selfupdate

import (
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandOver_MissingBinary_ReturnsError(t *testing.T) {
	err := handOver(filepath.Join(t.TempDir(), "not-downloaded.exe"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "selfupdate: relaunch: start")
}

// The successor must leave this process's group and console behind, otherwise
// the same console signal that stops quiver stops its replacement with it.
// CREATE_NEW_PROCESS_GROUP is 0x200 and DETACHED_PROCESS 0x8; the combination
// is the whole reason no separate helper binary is needed.
func TestHandOver_DetachesTheSuccessor(t *testing.T) {
	assert.Equal(t, uint32(0x00000208), uint32(syscall.CREATE_NEW_PROCESS_GROUP|detachedProcess))
}
