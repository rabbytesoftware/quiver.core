package api

import (
	"sync"

	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

// WSVersion is the interface each API version's WS handler must implement.
type WSVersion interface {
	PushArrow(domain.Arrow)
	PushArrowRuntime(domainRuntime.ArrowRuntime)
	PushQuiver(domain.Quiver)
}

// Hub fans out domain broadcasts to all registered API version WS handlers.
// It implements apphub.WebSocketHub so the app layer can broadcast through it.
type Hub struct {
	mu       sync.RWMutex
	versions []WSVersion
}

func NewHub() *Hub {
	return &Hub{}
}

// Register adds a WS version handler to the fan-out set.
func (h *Hub) Register(v WSVersion) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.versions = append(h.versions, v)
}

func (h *Hub) BroadcastArrow(arrow domain.Arrow) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, v := range h.versions {
		v.PushArrow(arrow)
	}
}

func (h *Hub) BroadcastArrowRuntime(rt domainRuntime.ArrowRuntime) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, v := range h.versions {
		v.PushArrowRuntime(rt)
	}
}

func (h *Hub) BroadcastQuiver(quiver domain.Quiver) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, v := range h.versions {
		v.PushQuiver(quiver)
	}
}
