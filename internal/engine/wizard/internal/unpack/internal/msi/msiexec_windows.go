//go:build windows

package msi

import (
	"context"
	"errors"
	"os/exec"
	"syscall"
)

func msiexec(
	ctx context.Context,
	inv invocation,
) (int, error) {
	cmd := exec.CommandContext(ctx, "msiexec")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: inv.commandLine()}

	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), nil
	}

	return exitSuccess, err
}
