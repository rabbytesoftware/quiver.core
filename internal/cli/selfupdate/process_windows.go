//go:build windows

package selfupdate

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

func alive(pid int) bool {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid)) // #nosec G115 -- a pid from the OS fits in 32 bits
	if err != nil {
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer windows.CloseHandle(handle) //nolint:errcheck // nothing to do about a failed close

	state, err := windows.WaitForSingleObject(handle, 0)
	return err == nil && state == uint32(windows.WAIT_TIMEOUT)
}

// detachAttempts detaches from the console and the parent's process group, and
// first tries to leave the parent's job object, which would otherwise end the
// update together with the run step that started it. A job that forbids
// breakaway refuses that, so the plainer detach follows. Not exercised on a
// real Windows machine.
func detachAttempts() []*syscall.SysProcAttr {
	const detached = windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP
	return []*syscall.SysProcAttr{
		{CreationFlags: detached | windows.CREATE_BREAKAWAY_FROM_JOB},
		{CreationFlags: detached},
	}
}
