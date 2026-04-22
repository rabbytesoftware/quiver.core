package ws

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	pingInterval = 30 * time.Second
	pongTimeout  = 60 * time.Second
	writeTimeout = 10 * time.Second
	sendBuffer   = 64
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type client struct {
	send chan []byte
	done chan struct{}
}

func newClient() *client {
	return &client{
		send: make(chan []byte, sendBuffer),
		done: make(chan struct{}),
	}
}

func readPump(conn *websocket.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(pongTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongTimeout))
	})
	for {
		if _, _, err := conn.NextReader(); err != nil {
			return
		}
	}
}

func writePump(conn *websocket.Conn, cl *client) {
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
		_ = conn.Close()
	}()
	for {
		select {
		case msg := <-cl.send:
			_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-cl.done:
			return
		}
	}
}
