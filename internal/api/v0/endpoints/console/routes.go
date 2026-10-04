package console

import (
	"github.com/gin-gonic/gin"

	consolehandlers "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/console/handlers"
	"github.com/rabbytesoftware/quiver.core/internal/console/command"
	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

// Register mounts the console routes: the live log stream, the list of
// commands the console may run, and command execution. The caller is expected
// to have placed rg behind the bearer-token gate.
func Register(
	rg *gin.RouterGroup,
	logs logring.Ring,
	exec command.Executor,
	opts ...consolehandlers.Option,
) {
	h := consolehandlers.New(logs, exec, opts...)
	rg.GET("/console/logs", h.Logs)
	rg.GET("/console/commands", h.Commands)
	rg.POST("/console/exec", h.Exec)
}
