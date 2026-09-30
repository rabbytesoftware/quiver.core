package ws

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

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

// Option configures a Handler.
type Option func(*options)

type options struct {
	jobs usecases.DiscoveryUsecase
}

// WithDiscoveryJobs gives the discovery stream the job registry it replays
// from and learns completion from. Without it the stream is live-only and never
// closes on its own.
func WithDiscoveryJobs(
	jobs usecases.DiscoveryUsecase,
) Option {
	return func(o *options) {
		o.jobs = jobs
	}
}

func NewHandler(
	opts ...Option,
) *Handler {
	var cfg options
	for _, opt := range opts {
		opt(&cfg)
	}

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
			// Runtime events carry the catalogued namespace, which always has a
			// ref, while a client subscribes with the namespace a user typed.
			// NamespaceMatch is what lets a refless subscription see them.
			KeyMatch: apiws.NamespaceMatch,
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
		Discovery: apiws.NewBroadcaster(discoveryDef(cfg.jobs)),
	}
}

// discoveryDef describes the result stream of one job. Each result is delivered
// exactly once and cannot be recovered from the wire afterwards, so a
// subscriber too slow to hold it is disconnected rather than left with a silent
// hole, and a reconnect is replayed from the job's own buffer.
func discoveryDef(
	jobs usecases.DiscoveryUsecase,
) apiws.StreamDef[usecases.StreamItem] {
	def := apiws.StreamDef[usecases.StreamItem]{
		Overflow: apiws.OverflowDisconnect,
		Seq: func(item usecases.StreamItem) uint64 {
			return item.Seq
		},
		Filters: []apiws.FilterDef[usecases.StreamItem]{
			{
				Param: "os",
				Extract: func(item usecases.StreamItem) string {
					return strings.Join(streamedOS(item), ",")
				},
				Match: containsOS,
			},
		},
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
	}

	if jobs != nil {
		def.Replay = jobs.Replay
		def.Done = jobs.Done
	}
	return def
}

func streamedOS(
	item usecases.StreamItem,
) []string {
	oses := make([]string, 0, len(item.Result.Arrow.Targets))
	for os := range item.Result.Arrow.Targets {
		oses = append(oses, string(os))
	}
	return oses
}

// containsOS applies the platform filter to a streamed result under the same
// rule GET /v0/search uses: an arrow is compatible with a platform when its
// compiled targets include it, and a filter that names no known platform
// selects nothing.
func containsOS(
	param string,
	value string,
) bool {
	return slices.Contains(strings.Split(value, ","), param)
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
