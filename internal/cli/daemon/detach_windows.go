//go:build windows

package daemon

import "syscall"

// detachAttrs is the Windows counterpart of the unix Setsid call.
// CREATE_NEW_PROCESS_GROUP detaches the child from the console's Ctrl+C, which
// is the closest equivalent to a new session.
//
// This exists so the package compiles for windows. The CLI is not usable there
// yet: the daemon transport is a unix socket throughout, and Windows needs a
// named pipe. See issue #214.
func detachAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
