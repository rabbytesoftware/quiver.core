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
// There is no executable bit to set on windows, so this is the same spawn the
// fallback performs. The two stay separate names because the unix pair are
// genuinely different operations and Relaunch calls them for different reasons.
func handOver(
	binPath string,
) error {
	return resume(binPath)
}

// resume spawns a binary already known to be runnable — the fallback target
// when a handover failed.
//
// CREATE_NEW_PROCESS_GROUP and DETACHED_PROCESS together are what make the
// successor outlive this process instead of dying with the console or job
// object it was started from — the exact failure a separate helper binary
// would otherwise have to exist to work around.
func resume(
	binPath string,
) error {
	cmd := exec.Command(binPath, os.Args[1:]...) //nolint:gosec // binPath is either quiver's own update artifact or the binary already running, never caller input
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("selfupdate: relaunch: start %s: %w", binPath, err)
	}

	return nil
}
