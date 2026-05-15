package hub

import (
	"sync"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

type CatalogEventKind int

const (
	CatalogUpserted CatalogEventKind = iota
	CatalogRemoved
)

type ArrowEvent struct {
	Kind CatalogEventKind
	domain.Arrow
}

type CollectionEvent struct {
	Kind CatalogEventKind
	domain.Collection
}

// WebSocketHub is the version-agnostic interface for broadcasting domain
// aggregates to connected WebSocket clients. Defined here (app layer) so
// app builders can depend on it without importing the api layer.
type WebSocketHub interface {
	BroadcastArrow(ArrowEvent)
	BroadcastArrowRuntime(runtime domainRuntime.ArrowRuntime)
	BroadcastCollection(CollectionEvent)
}

// Subscriber receives domain broadcasts. Implemented by API version WS handlers.
type Subscriber interface {
	PushArrow(ArrowEvent)
	PushArrowRuntime(domainRuntime.ArrowRuntime)
	PushCollection(CollectionEvent)
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

func (h *Hub) BroadcastArrow(evt ArrowEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushArrow(evt)
	}
}

func (h *Hub) BroadcastArrowRuntime(rt domainRuntime.ArrowRuntime) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushArrowRuntime(rt)
	}
}

func (h *Hub) BroadcastCollection(evt CollectionEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subscribers {
		s.PushCollection(evt)
	}
}
