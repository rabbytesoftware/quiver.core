//go:build darwin || linux

package launch

import (
	"os/exec"
	"syscall"
)

func detach(
	cmd *exec.Cmd,
	_ string,
	_ bool,
) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func breakawayDenied(
	_ error,
) bool {
	return false
}
