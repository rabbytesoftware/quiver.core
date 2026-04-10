package arrows

import (
	"github.com/gin-gonic/gin"
	arrowhandlers "github.com/rabbytesoftware/quiver/internal/api/v1/endpoints/arrows/handlers"
	"github.com/rabbytesoftware/quiver/internal/app/arrow"
)

// WSHandler is the interface satisfied by the v1 WS handler for arrow channels.
type WSHandler interface {
	HandleArrow(c *gin.Context)
	HandleArrowRuntime(c *gin.Context)
}

// Register mounts all /arrow routes onto the given router group.
// GET /arrow and GET /arrow/:ns dispatch to WS on Upgrade header, REST otherwise.
// GET /arrow.runtime and GET /arrow.runtime/:ns are pure WS endpoints.
func Register(
	rg *gin.RouterGroup,
	svc arrow.ArrowService,
	wsHandler WSHandler,
) {
	h := arrowhandlers.New(svc)
	rg.POST("/arrow/:ns", h.Add)
	rg.PATCH("/arrow/:ns", h.Update)
	rg.DELETE("/arrow/:ns", h.Remove)
	rg.GET("/arrow", dispatch(h.List, wsHandler.HandleArrow))
	rg.GET("/arrow/:ns", dispatch(h.GetDetail, wsHandler.HandleArrow))
	rg.POST("/arrow/:ns/:method", h.Execute)

	// arrow.runtime — pure WS, no REST counterpart
	rg.GET("/arrow.runtime", wsHandler.HandleArrowRuntime)
	rg.GET("/arrow.runtime/:ns", wsHandler.HandleArrowRuntime)
}

// dispatch checks the Upgrade header. WS requests go to wsHandler; plain HTTP goes to rest.
func dispatch(rest, ws gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Upgrade") == "websocket" {
			ws(c)
			return
		}
		rest(c)
	}
}
