//go:build windows

package selfupdate

import (
	"errors"

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
