package v0

import (
	"github.com/rabbytesoftware/quiver.core/internal/console/command"
	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

type options struct {
	consoleLogs logring.Ring
	consoleExec command.Executor
}
