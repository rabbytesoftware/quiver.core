package shelf

import (
	"context"
	"os"
	"os/exec"
)

type execCommander struct{}

var defaultCommander Commander = &execCommander{}

func (*execCommander) Run(
	ctx context.Context,
	name string,
	args ...string,
) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput() //nolint:gosec
}

func (*execCommander) RunWithEnv(
	ctx context.Context,
	env []string,
	name string,
	args ...string,
) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec
	cmd.Env = append(os.Environ(), env...)
	return cmd.CombinedOutput()
}
