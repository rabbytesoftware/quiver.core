package ws_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rabbytesoftware/quiver/internal/api/v1/ws"
	"github.com/stretchr/testify/assert"
)

func TestWebSocketEcho(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	handler := ws.NewMockWebSocketHandler()
	r.GET("/ws", func(c *gin.Context) {
		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		conn, _ := upgrader.Upgrade(c.Writer, c.Request, nil)
		go handler.HandleConnection(conn)
	})

	server := httptest.NewServer(r)
	defer server.Close()

	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	assert.NoError(t, err)
	defer conn.Close()

	err = conn.WriteJSON(ws.NewMessage(ws.MessageEcho, "hello"))
	assert.NoError(t, err)

	var resp ws.Message
	err = conn.ReadJSON(&resp)
	assert.NoError(t, err)
	assert.Equal(t, ws.MessageEcho, resp.Type)
	assert.Equal(t, "hello", resp.Payload)
}
