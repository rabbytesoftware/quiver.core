package console

import (
	"github.com/gin-gonic/gin"

	consolehandlers "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/console/handlers"
	"github.com/rabbytesoftware/quiver.core/internal/core/logring"
)

func Register(
	rg *gin.RouterGroup,
	logs *logring.Ring,
	version string,
) {
	h := consolehandlers.New(logs, version)
	rg.GET("/console/logs", h.Logs)
	rg.GET("/console/commands", h.Commands)
	rg.POST("/console/exec", h.Exec)
}
