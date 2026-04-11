package api

import (
	"sync"

	apiv0 "github.com/rabbytesoftware/quiver/internal/api/v0"
	wshandler "github.com/rabbytesoftware/quiver/internal/api/v0/ws"
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
// Use NewHub to obtain an instance — it pre-registers all current versions.
type Hub struct {
	mu       sync.RWMutex
	versions []WSVersion
	v0       *wshandler.Handler // used by Init to mount v0 routes
}

// NewHub creates the hub and registers all current API version WS handlers.
// The returned value is passed to both app.Init (for broadcast subscriptions)
// and api.Init (for route mounting). Adding a new API version means adding
// it here and in Init — internal.go never changes.
func NewHub() *Hub {
	ws0 := apiv0.NewWSHandler()
	h := &Hub{v0: ws0}
	h.versions = append(h.versions, ws0)
	return h
}

// Register adds an additional WS version to the fan-out set.
// Intended for future API versions or test injection.
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
