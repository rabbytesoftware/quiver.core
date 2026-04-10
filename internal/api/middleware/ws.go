// internal/api/middleware/ws.go
package middleware

import (
	"net/http"

	"github.com/gorilla/websocket"
)

// Upgrader is the shared WebSocket upgrader. Accepts all origins (v0 — no auth).
var Upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}
