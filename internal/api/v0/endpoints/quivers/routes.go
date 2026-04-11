package quivers

import (
	"strings"

	"github.com/gin-gonic/gin"
	quiverhandlers "github.com/rabbytesoftware/quiver/internal/api/v0/endpoints/quivers/handlers"
	appquiver "github.com/rabbytesoftware/quiver/internal/app/quiver"
)

type WSHandler interface {
	HandleQuiver(c *gin.Context)
}

// Register mounts all /quiver routes onto the given router group.
// GET /quiver and GET /quiver/:ns dispatch to WS on Upgrade header, REST otherwise.
func Register(
	rg *gin.RouterGroup,
	svc appquiver.QuiverService,
	wsHandler WSHandler,
) {
	h := quiverhandlers.New(svc)
	rg.POST("/quiver/:ns", h.Add)
	rg.PATCH("/quiver/:ns", h.Update)
	rg.DELETE("/quiver/:ns", h.Remove)
	rg.GET("/quiver", dispatch(h.List, wsHandler.HandleQuiver))
	rg.GET("/quiver/:ns", dispatch(h.Get, wsHandler.HandleQuiver))
}

func dispatch(rest, ws gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
			ws(c)
			return
		}
		rest(c)
	}
}
