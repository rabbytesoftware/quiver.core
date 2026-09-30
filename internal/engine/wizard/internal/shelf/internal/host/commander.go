package host

import (
	"context"
	"os/exec"
)

type Commander interface {
	Run(
		ctx context.Context,
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
	return exec.CommandContext(ctx, name, args...).CombinedOutput() // #nosec G204 -- commander runs fixed OS tools (codesign) chosen by the shelf, never manifest input
}
