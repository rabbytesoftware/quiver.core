package platform

import (
	"context"
	"os"
	"os/exec"
)

type Commander interface {
	Run(
		ctx context.Context,
		name string,
		args ...string,
	) ([]byte, error)
	RunWithEnv(
		ctx context.Context,
		env []string,
		name string,
		args ...string,
	) ([]byte, error)
}

type execCommander struct{}

func NewCommander() Commander {
	return &execCommander{}
}

func (*execCommander) Run(
	ctx context.Context,
	name string,
	args ...string,
) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput() // #nosec G204 -- commander runs fixed OS tools (codesign, powershell) chosen by shelf, never manifest input
}

func (*execCommander) RunWithEnv(
	ctx context.Context,
	env []string,
	name string,
	args ...string,
) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204 -- commander runs fixed OS tools (codesign, powershell) chosen by shelf, never manifest input
	cmd.Env = append(os.Environ(), env...)
	return cmd.CombinedOutput()
}
