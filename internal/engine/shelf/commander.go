package shelf

import (
	"context"
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
