package hub

import (
	"sync"

	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

// WebSocketHub is the version-agnostic interface for broadcasting domain
// aggregates to connected WebSocket clients. Defined here (app layer) so
// app builders can depend on it without importing the api layer.
type WebSocketHub interface {
	BroadcastArrow(arrow domain.Arrow)
	BroadcastArrowRuntime(runtime domainRuntime.ArrowRuntime)
	BroadcastQuiver(quiver domain.Quiver)
}

// Subscriber receives domain broadcasts. Implemented by API version WS handlers.
type Subscriber interface {
	PushArrow(domain.Arrow)
	PushArrowRuntime(domainRuntime.ArrowRuntime)
	PushQuiver(domain.Quiver)
}

// Hub fans out domain broadcasts to all registered Subscribers.
// It implements WebSocketHub so the app layer can broadcast through it.
type Hub struct {
	mu          sync.RWMutex
	subscribers []Subscriber
}

func NewHub() *Hub {
	return &Hub{}
}

func (h *Hub) Register(s Subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subscribers = append(h.subscribers, s)
}

func (h *Hub) BroadcastArrow(arrow domain.Arrow) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushArrow(arrow)
	}
}

func (h *Hub) BroadcastArrowRuntime(rt domainRuntime.ArrowRuntime) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushArrowRuntime(rt)
	}
}

func (h *Hub) BroadcastQuiver(quiver domain.Quiver) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushQuiver(quiver)
	}
}
