package ws

import (
	"encoding/json"
	"strconv"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	apiws "github.com/rabbytesoftware/quiver.core/internal/api/ws"
	apphub "github.com/rabbytesoftware/quiver.core/internal/app/hub"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

type Handler struct {
	Arrow      *apiws.Broadcaster[apphub.ArrowEvent]
	Runtime    *apiws.Broadcaster[domainRuntime.ArrowRuntime]
	Collection *apiws.Broadcaster[apphub.CollectionEvent]
	// Discovery carries verified search results and nothing else. Counts and
	// provider failures are read from the job resource, never streamed: a
	// stream is one payload type.
	Discovery *apiws.Broadcaster[usecases.StreamItem]
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
		Discovery: apiws.NewBroadcaster(apiws.StreamDef[usecases.StreamItem]{
			KeyParam: "job",
			// A job id is opaque, so it is compared literally. Globbing here
			// would let /v0/search/discover/* read every job's results.
			KeyMatch: apiws.ExactMatch,
			Namespace: func(item usecases.StreamItem) string {
				return item.JobID
			},
			Serialize: func(item usecases.StreamItem) ([]byte, error) {
				return json.Marshal(dto.SearchResultDTOFromDiscovery(item.Result))
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

// PushDiscovery fans one verified result out to the subscribers of its job. It
// is wired straight to the usecase's OnResult hook rather than through the
// domain hub: a discovery result is not a domain aggregate and has no
// projection behind it.
func (h *Handler) PushDiscovery(item usecases.StreamItem) {
	h.Discovery.Push(item)
}
