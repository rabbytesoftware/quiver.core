package api

import "github.com/rabbytesoftware/quiver/internal/domain"

// WebSocketHub is the version-agnostic interface for broadcasting domain aggregates
// to all connected WebSocket clients. The hub extracts the namespace from the aggregate
// and handles all internal routing (global channels, namespace channels, version fan-out).
//
// The interface is consumed by the app layer (dependency inversion). Callers pass the
// aggregate directly — no knowledge of channels, DTOs, or API versions required.
//
// Defined by websocket.md §7.
type WebSocketHub interface {
	BroadcastArrow(arrow domain.Arrow)
	BroadcastQuiver(quiver domain.Quiver)
	// BroadcastArrowRuntime will be added in Phase 1A once domain.ArrowRuntime is defined.
}
