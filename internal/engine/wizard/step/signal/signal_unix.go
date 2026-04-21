//go:build !windows

package signal

import (
	"os"
	"syscall"
)

var signalMap = map[string]os.Signal{
	// SignalKind domain values
	"graceful":  syscall.SIGTERM,
	"kill":      syscall.SIGKILL,
	"interrupt": syscall.SIGINT,
	// POSIX names
	"SIGTERM": syscall.SIGTERM,
	"SIGINT":  syscall.SIGINT,
	"SIGKILL": syscall.SIGKILL,
	"SIGHUP":  syscall.SIGHUP,
	"SIGUSR1": syscall.SIGUSR1,
	"SIGUSR2": syscall.SIGUSR2,
	"SIGCONT": syscall.SIGCONT,
}
