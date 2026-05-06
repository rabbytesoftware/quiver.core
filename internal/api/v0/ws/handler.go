package ws

import (
	"encoding/json"
	"strconv"

	"github.com/rabbytesoftware/quiver/internal/api/v0/dto"
	apiws "github.com/rabbytesoftware/quiver/internal/api/ws"
	"github.com/rabbytesoftware/quiver/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver/internal/domain/runtime"
)

type Handler struct {
	Arrow   *apiws.Broadcaster[domain.Arrow]
	Runtime *apiws.Broadcaster[domainRuntime.ArrowRuntime]
	Collection  *apiws.Broadcaster[domain.Collection]
}

func NewHandler() *Handler {
	return &Handler{
		Arrow: apiws.NewBroadcaster(apiws.StreamDef[domain.Arrow]{
			Namespace: func(a domain.Arrow) string {
				return string(a.Namespace)
			},
			Serialize: func(a domain.Arrow) ([]byte, error) {
				return json.Marshal(dto.ArrowDTOFrom(a))
			},
			Filters: []apiws.FilterDef[domain.Arrow]{
				{
					Param: "user_installed",
					Extract: func(a domain.Arrow) string {
						return strconv.FormatBool(a.UserInstalled)
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
		Collection: apiws.NewBroadcaster(apiws.StreamDef[domain.Collection]{
			Namespace: func(q domain.Collection) string {
				return string(q.Namespace)
			},
			Serialize: func(q domain.Collection) ([]byte, error) {
				return json.Marshal(dto.QuiverDTOFrom(q))
			},
		}),
	}
}

func (h *Handler) PushArrow(
	a domain.Arrow,
) {
	h.Arrow.Push(a)
}

func (h *Handler) PushArrowRuntime(
	rt domainRuntime.ArrowRuntime,
) {
	h.Runtime.Push(rt)
}

func (h *Handler) PushCollection(
	q domain.Collection,
) {
	h.Collection.Push(q)
}
