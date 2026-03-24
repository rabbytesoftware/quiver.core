package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver/internal/api/v1/ws"
)

type mockWSHandler struct {
	called chan struct{}
}

func (m *mockWSHandler) HandleConnection(_ *websocket.Conn) {
	m.called <- struct{}{}
}

func (m *mockWSHandler) Broadcast(_ ws.Message)               {}
func (m *mockWSHandler) Send(_ *websocket.Conn, _ ws.Message) {}

func TestWebSocketUpgrade_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &mockWSHandler{
		called: make(chan struct{}, 1),
	}

	router := gin.New()
	router.GET("/ws", WebSocketUpgrade(handler))

	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + server.URL[len("http"):]

	conn, _, err := websocket.DefaultDialer.Dial(wsURL+"/ws", nil)
	assert.NoError(t, err)
	defer conn.Close()

	select {
	case <-handler.called:
		// success
	case <-time.After(time.Second):
		t.Fatal("HandleConnection was not called")
	}
}

func TestWebSocketUpgrade_Failure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &mockWSHandler{
		called: make(chan struct{}, 1),
	}

	router := gin.New()
	router.GET("/ws", WebSocketUpgrade(handler))

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	select {
	case <-handler.called:
		t.Fatal("HandleConnection should not be called on upgrade failure")
	default:
		// correct behavior
	}
}
