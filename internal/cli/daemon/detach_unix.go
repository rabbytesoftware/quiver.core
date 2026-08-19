//go:build !windows

package daemon

import "syscall"

// detachAttrs puts the daemon in its own session so it outlives the CLI
// process that started it and does not receive the terminal's signals.
func detachAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
