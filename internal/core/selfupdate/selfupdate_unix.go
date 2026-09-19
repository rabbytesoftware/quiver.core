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

	if err := syscall.Exec(binPath, os.Args, os.Environ()); err != nil { //nolint:gosec // binPath is quiver's own release binary, fetched and checksummed by its own update lifecycle, not caller input
		return fmt.Errorf("selfupdate: relaunch: exec %s: %w", binPath, err)
	}

	return nil
}
