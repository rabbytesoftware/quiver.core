package console

import (
	"github.com/gin-gonic/gin"

	consolehandlers "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/console/handlers"
	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

// Register mounts the three console routes on rg: GET /console/logs (a
// WebSocket of the daemon's log records read from logs), GET /console/commands
// (the commands the console may run) and POST /console/exec (run one of them
// and stream its output). version is what the console's CLI reports as its own.
// The caller places rg behind the bearer-token gate, so these routes need no
// authentication of their own.
func Register(
	rg *gin.RouterGroup,
	logs logring.Ring,
	version string,
) {
	h := consolehandlers.New(logs, version)
	rg.GET("/console/logs", h.Logs)
	rg.GET("/console/commands", h.Commands)
	rg.POST("/console/exec", h.Exec)
}
