// Package console serves the daemon console: the live log stream, the list of
// commands the console may run, and command execution. See
// docs/spec/console.md for the contract.
package console

import (
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/console/command"
	"github.com/rabbytesoftware/quiver.core/internal/console/logring"
)

const (
	defaultExecTimeout = 10 * time.Minute
	defaultOutputLimit = 256 * 1024
	defaultPerDevice   = 2
	defaultGlobal      = 8
	defaultStuckLimit  = 5 * time.Second
)

// Handlers serves the console endpoints.
type Handlers struct {
	logs        logring.Ring
	exec        command.Executor
	limiter     ExecLimiter
	execTimeout time.Duration
	outputLimit int
	stuckLimit  time.Duration
}

// New returns Handlers reading logs from logs and running commands through
// exec. Without options it applies the contract's limits: a 10 minute command
// timeout, 256 KiB of output per call, 2 concurrent commands per device and 8
// overall, and a 5 second tolerance for a log consumer that stops reading.
func New(
	logs logring.Ring,
	exec command.Executor,
	opts ...Option,
) *Handlers {
	h := &Handlers{
		logs:        logs,
		exec:        exec,
		limiter:     NewExecLimiter(defaultPerDevice, defaultGlobal),
		execTimeout: defaultExecTimeout,
		outputLimit: defaultOutputLimit,
		stuckLimit:  defaultStuckLimit,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}
