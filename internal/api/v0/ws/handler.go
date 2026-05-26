package ws

import (
	"encoding/json"
	"strconv"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	apiws "github.com/rabbytesoftware/quiver.core/internal/api/ws"
	apphub "github.com/rabbytesoftware/quiver.core/internal/app/hub"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

type Handler struct {
	Arrow      *apiws.Broadcaster[apphub.ArrowEvent]
	Runtime    *apiws.Broadcaster[domainRuntime.ArrowRuntime]
	Collection *apiws.Broadcaster[apphub.CollectionEvent]
}

func NewHandler() *Handler {
	return &Handler{
		Arrow: apiws.NewBroadcaster(apiws.StreamDef[apphub.ArrowEvent]{
			Namespace: func(e apphub.ArrowEvent) string {
				return string(e.Namespace)
			},
			Serialize: func(e apphub.ArrowEvent) ([]byte, error) {
				return json.Marshal(dto.ArrowEventDTOFrom(e))
			},
			Filters: []apiws.FilterDef[apphub.ArrowEvent]{
				{
					Param: "user_installed",
					Extract: func(e apphub.ArrowEvent) string {
						return strconv.FormatBool(e.UserInstalled)
					},
					Match:   apiws.ExactMatch,
					Default: "true",
				},
			},
		}),
		Runtime: apiws.NewBroadcaster(apiws.StreamDef[domainRuntime.ArrowRuntime]{
			Namespace: func(rt domainRuntime.ArrowRuntime) string {
				return rt.Ref.String()
			},
			Serialize: func(rt domainRuntime.ArrowRuntime) ([]byte, error) {
				return json.Marshal(dto.ArrowRuntimeDTOFrom(rt))
			},
		}),
		Collection: apiws.NewBroadcaster(apiws.StreamDef[apphub.CollectionEvent]{
			Namespace: func(e apphub.CollectionEvent) string {
				return string(e.Namespace)
			},
			Serialize: func(e apphub.CollectionEvent) ([]byte, error) {
				return json.Marshal(dto.CollectionEventDTOFrom(e))
			},
		}),
	}
}

func (h *Handler) PushArrow(e apphub.ArrowEvent) {
	h.Arrow.Push(e)
}

func (h *Handler) PushArrowRuntime(rt domainRuntime.ArrowRuntime) {
	h.Runtime.Push(rt)
}

func (h *Handler) PushCollection(e apphub.CollectionEvent) {
	h.Collection.Push(e)
}
