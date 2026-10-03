//go:build windows

package daemon

import "syscall"

// DetachAttrs is the Windows counterpart of the unix Setsid call.
// CREATE_NEW_PROCESS_GROUP detaches the child from the console's Ctrl+C, which
// is the closest equivalent to a new session.
func DetachAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
