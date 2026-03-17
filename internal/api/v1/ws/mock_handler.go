package ws

import (
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

type MockWebSocketHandler struct {
	connections map[*websocket.Conn]struct{}
	mu          sync.RWMutex
}

func NewMockWebSocketHandler() *MockWebSocketHandler {
	return &MockWebSocketHandler{
		connections: make(map[*websocket.Conn]struct{}),
	}
}

func (h *MockWebSocketHandler) HandleConnection(conn *websocket.Conn) {
	h.addConnection(conn)
	defer h.removeConnection(conn)

	for {
		var msg Message
		if err := conn.ReadJSON(&msg); err != nil {
			log.Println("read error:", err)
			return
		}

		switch msg.Type {
		case MessagePing:
			h.Send(conn, NewMessage(MessagePong, "pong"))
		default:
			h.Send(conn, NewMessage(MessageEcho, msg.Payload))
		}
	}
}

func (h *MockWebSocketHandler) Broadcast(message Message) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for conn := range h.connections {
		_ = conn.WriteJSON(message)
	}
}

func (h *MockWebSocketHandler) Send(conn *websocket.Conn, message Message) {
	_ = conn.WriteJSON(message)
}

func (h *MockWebSocketHandler) addConnection(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.connections[conn] = struct{}{}
}

func (h *MockWebSocketHandler) removeConnection(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.connections, conn)
	_ = conn.Close()
}
