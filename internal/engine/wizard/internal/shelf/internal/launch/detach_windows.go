//go:build windows

package launch

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
	createBreakaway       = 0x01000000
)

func detach(
	cmd *exec.Cmd,
	target string,
	breakaway bool,
) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: creationFlags(target, breakaway)}
}

// creationFlags detaches a program from the daemon's console. A batch script
// runs under cmd.exe, which would open a console window of its own when
// detached, so it is created without one instead. Breakaway lets the program
// outlive a job object the daemon was started in.
func creationFlags(
	target string,
	breakaway bool,
) uint32 {
	flags := uint32(createNewProcessGroup)
	switch strings.ToLower(filepath.Ext(target)) {
	case ".bat", ".cmd":
		flags |= createNoWindow
	default:
		flags |= detachedProcess
	}
	if breakaway {
		flags |= createBreakaway
	}
	return flags
}

func breakawayDenied(
	err error,
) bool {
	return errors.Is(err, syscall.ERROR_ACCESS_DENIED)
}
