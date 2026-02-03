package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rabbytesoftware/quiver/internal/ws"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func WebSocketUpgrade(handler ws.WebSocketHandler) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}

		go handler.HandleConnection(conn)
	}
}
