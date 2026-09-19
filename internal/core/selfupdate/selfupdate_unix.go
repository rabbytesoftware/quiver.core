//go:build darwin || linux

package selfupdate

import (
	"fmt"
	"os"
	"syscall"
)

// executableMode is the mode the downloaded binary is given before it is
// exec'd. The fetch step writes it through os.Create, which lands at 0644 —
// not executable — so without this the handover fails with EACCES on a file
// that downloaded perfectly.
const executableMode = 0o755

// handOver replaces the current process image with binPath, preserving the PID
// and every inherited file descriptor. There is no separate stop step: on unix
// the exec is the stop, for this process. Everything that had to be released
// was released by the shutdown sequence that ran before Relaunch was called.
func handOver(
	binPath string,
) error {
	if err := os.Chmod(binPath, executableMode); err != nil { //nolint:gosec // an executable the process is about to become must carry the executable bit
		return fmt.Errorf("selfupdate: relaunch: chmod %s: %w", binPath, err)
	}

	return resume(binPath)
}

// resume execs a binary that is already known to be executable, and so does
// not chmod it. That distinction is the whole reason it is separate from
// handOver: the fallback target is routinely a root-owned 0755 binary while
// the daemon runs unprivileged, where a chmod would return EPERM and strand a
// machine that still had a perfectly good quiver on disk.
func resume(
	binPath string,
) error {
	if err := syscall.Exec(binPath, os.Args, os.Environ()); err != nil { //nolint:gosec // binPath is either quiver's own update artifact or the binary already running, never caller input
		return fmt.Errorf("selfupdate: relaunch: exec %s: %w", binPath, err)
	}

	return nil
}
