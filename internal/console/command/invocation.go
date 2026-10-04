package command

import (
	"context"
	"io"
)

// Invocation is one authorized command ready to run once.
type Invocation interface {
	// Run executes the command, writing its output to stdout and stderr, and
	// returns when it finishes or ctx is done. A panic inside the command is
	// recovered into a failed Result.
	//
	// Output written after Run returns, by a command that ignored ctx, must be
	// discarded by the writers.
	Run(
		ctx context.Context,
		stdout io.Writer,
		stderr io.Writer,
	) Result
}
