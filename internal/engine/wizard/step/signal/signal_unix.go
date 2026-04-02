//go:build !windows

package signal

import "syscall"

func init() {
	signalMap["SIGHUP"] = syscall.SIGHUP
	signalMap["SIGUSR1"] = syscall.SIGUSR1
	signalMap["SIGUSR2"] = syscall.SIGUSR2
}
