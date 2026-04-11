package hub

import (
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
