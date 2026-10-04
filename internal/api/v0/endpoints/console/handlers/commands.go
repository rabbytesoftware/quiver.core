package console

import (
	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
)

// Commands lists the commands the console may run.
//
// @Summary      List the commands the console may run
// @Description  Returns every command, with its usage and flags, that carries the console annotation and whose parent chain does too. Clients render help and completion from this list instead of keeping their own.
// @Tags         console
// @Produce      json
// @Success      200  {object}  libs.QueryResponse{data=commandsResponse}
// @Router       /console/commands [get]
func (h *Handlers) Commands(c *gin.Context) {
	libs.WriteQueryOK(c, commandsResponse{Commands: h.exec.Commands()})
}
