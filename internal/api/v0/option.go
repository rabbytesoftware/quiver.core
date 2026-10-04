package v0

import (
	"github.com/rabbytesoftware/quiver.core/internal/console/command"
	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

// Option configures the v0 container.
type Option func(*options)

// WithConsole mounts the console routes, serving logs from the given ring and
// running commands through exec. Without it the container serves no console
// routes. internal.New builds both, so the daemon's logger, the log stream and
// the executor all share one ring and one address.
func WithConsole(
	logs logring.Ring,
	exec command.Executor,
) Option {
	return func(o *options) {
		o.consoleLogs = logs
		o.consoleExec = exec
	}
}
