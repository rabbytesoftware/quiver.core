//go:build windows

package selfupdate

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// detachedProcess is the DETACHED_PROCESS creation flag. Go's syscall package
// does not export it.
const detachedProcess = 0x00000008

// handOver spawns binPath as a new, detached process and returns; the caller
// exits normally afterwards. Windows has no exec-style same-process
// replacement, so unlike the unix path this changes PID — nothing in
// self-succession relies on PID continuity across an update, and the arrow's
// own supervised processes are not children of this one.
//
// CREATE_NEW_PROCESS_GROUP and DETACHED_PROCESS together are what make the
// successor outlive this process instead of dying with the console or job
// object it was started from — the exact failure a separate helper binary
// would otherwise have to exist to work around.
func handOver(
	binPath string,
) error {
	cmd := exec.Command(binPath, os.Args[1:]...) //nolint:gosec // binPath is quiver's own downloaded release binary, not caller input
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("selfupdate: relaunch: start %s: %w", binPath, err)
	}

	return nil
}
