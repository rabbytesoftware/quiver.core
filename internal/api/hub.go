package api

import (
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

// WSVersion is the interface that each API version's WS handler must implement.
// The hub fans out domain broadcasts to all registered versions.
type WSVersion interface {
	PushArrow(domain.Arrow)
	PushArrowRuntime(domainRuntime.ArrowRuntime)
	PushQuiver(domain.Quiver)
}

type hub struct {
	versions []WSVersion
}

func NewHub(versions ...WSVersion) *hub {
	return &hub{versions: versions}
}

func (h *hub) BroadcastArrow(arrow domain.Arrow) {
	for _, v := range h.versions {
		v.PushArrow(arrow)
	}
}

func (h *hub) BroadcastArrowRuntime(rt domainRuntime.ArrowRuntime) {
	for _, v := range h.versions {
		v.PushArrowRuntime(rt)
	}
}

func (h *hub) BroadcastQuiver(quiver domain.Quiver) {
	for _, v := range h.versions {
		v.PushQuiver(quiver)
	}
}
