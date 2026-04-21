package ws

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rabbytesoftware/quiver/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

const (
	pingInterval = 30 * time.Second
	pongTimeout  = 60 * time.Second
	writeTimeout = 10 * time.Second
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type channelKey struct {
	kind      string // "arrow" | "arrow.runtime" | "quiver"
	namespace string // empty = global channel
}

type client struct {
	send chan []byte
	done chan struct{}
}

type Handler struct {
	mu         sync.RWMutex
	clients    map[channelKey]map[*client]struct{}
	registered chan struct{}
	once       sync.Once
}

func NewHandler() *Handler {
	return &Handler{
		clients:    make(map[channelKey]map[*client]struct{}),
		registered: make(chan struct{}),
	}
}

func (h *Handler) WaitRegistered() {
	<-h.registered
}

func (h *Handler) HandleArrow(c *gin.Context) {
	h.handle(c, "arrow")
}

func (h *Handler) HandleArrowRuntime(c *gin.Context) {
	h.handle(c, "arrow.runtime")
}

func (h *Handler) HandleQuiver(c *gin.Context) {
	h.handle(c, "quiver")
}

func (h *Handler) handle(c *gin.Context, kind string) {
	ns := c.Param("ns") // empty for global routes
	key := channelKey{kind: kind, namespace: ns}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	cl := &client{
		send: make(chan []byte, 64),
		done: make(chan struct{}),
	}
	h.register(key, cl)

	go h.writePump(conn, cl)
	h.readPump(conn, cl)
	h.unregister(key, cl)
}

func (h *Handler) register(key channelKey, cl *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[key] == nil {
		h.clients[key] = make(map[*client]struct{})
	}
	h.clients[key][cl] = struct{}{}
	h.once.Do(func() { close(h.registered) })
}

func (h *Handler) unregister(key channelKey, cl *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients[key], cl)
	close(cl.done)
}

func (h *Handler) readPump(conn *websocket.Conn, _ *client) {
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

func (h *Handler) writePump(conn *websocket.Conn, cl *client) {
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

func (h *Handler) broadcast(kind, namespace string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()

	for cl := range h.clients[channelKey{kind: kind, namespace: ""}] {
		select {
		case cl.send <- data:
		default:
		}
	}

	if namespace != "" {
		for cl := range h.clients[channelKey{kind: kind, namespace: namespace}] {
			select {
			case cl.send <- data:
			default:
			}
		}
	}
}

func (h *Handler) PushArrow(arrow domain.Arrow) {
	h.broadcast("arrow", string(arrow.Namespace), dto.ArrowDTOFrom(arrow))
}

func (h *Handler) PushArrowRuntime(rt domainRuntime.ArrowRuntime) {
	h.broadcast("arrow.runtime", rt.Ref.String(), dto.ArrowRuntimeDTOFrom(rt))
}

func (h *Handler) PushQuiver(quiver domain.Quiver) {
	h.broadcast("quiver", string(quiver.Namespace), dto.QuiverDTOFrom(quiver))
}
