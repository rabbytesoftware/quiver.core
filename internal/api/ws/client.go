package ws

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	pingInterval = 30 * time.Second
	pongTimeout  = 60 * time.Second
	writeTimeout = 10 * time.Second
	sendBuffer   = 64
)

const (
	closeReasonCompleted = "completed"
	closeReasonSlow      = "slow consumer"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type client struct {
	send chan []byte
	done chan struct{}
	// dead is closed by writePump on exit, whatever ended it, so anything
	// blocked on this client stops waiting for a peer that is gone.
	dead chan struct{}
	// finish asks writePump to flush what is queued and close with a normal
	// closure; kick asks it to close with try-again-later.
	finish     chan struct{}
	kick       chan struct{}
	finishOnce sync.Once
	kickOnce   sync.Once
}

func newClient() *client {
	return &client{
		send:   make(chan []byte, sendBuffer),
		done:   make(chan struct{}),
		dead:   make(chan struct{}),
		finish: make(chan struct{}),
		kick:   make(chan struct{}),
	}
}

func (c *client) requestFinish() {
	c.finishOnce.Do(func() { close(c.finish) })
}

func (c *client) requestKick() {
	c.kickOnce.Do(func() { close(c.kick) })
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
		close(cl.dead)
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
		case <-cl.finish:
			flushAndClose(conn, cl, websocket.CloseNormalClosure, closeReasonCompleted)
			return
		case <-cl.kick:
			writeClose(conn, websocket.CloseTryAgainLater, closeReasonSlow)
			return
		case <-cl.done:
			return
		}
	}
}

func flushAndClose(
	conn *websocket.Conn,
	cl *client,
	code int,
	reason string,
) {
	for {
		select {
		case msg := <-cl.send:
			_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		default:
			writeClose(conn, code, reason)
			return
		}
	}
}

func writeClose(
	conn *websocket.Conn,
	code int,
	reason string,
) {
	_ = conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(code, reason),
		time.Now().Add(writeTimeout),
	)
}
