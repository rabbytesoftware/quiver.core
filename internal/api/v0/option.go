package v0

import "github.com/rabbytesoftware/quiver.core/internal/core/logring"

// Option configures the v0 container.
type Option func(*options)

// WithConsole mounts the console routes: the live log stream served from logs,
// the list of commands the console may run, and command execution. version is
// what the console's CLI reports as its own. Without this option the container
// serves no console routes.
func WithConsole(
	logs logring.Ring,
	version string,
) Option {
	return func(
		o *options,
	) {
		o.consoleLogs = logs
		o.consoleVersion = version
	}
}
