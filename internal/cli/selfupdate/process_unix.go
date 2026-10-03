//go:build !windows

package selfupdate

import (
	"errors"
	"syscall"
)

func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// detachAttempts is the one way to detach on unix: a session of its own.
func detachAttempts() []*syscall.SysProcAttr {
	return []*syscall.SysProcAttr{{Setsid: true}}
}
