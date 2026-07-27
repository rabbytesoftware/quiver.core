package ws_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	ws "github.com/rabbytesoftware/quiver.core/internal/api/v0/ws"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// probeNamespace marks the throwaway results that prove a subscriber is
// registered; fenceNamespace marks the result that proves every earlier push
// has been delivered. Both are filtered out before anything is asserted.
const (
	probeNamespace = "probe.invalid/probe/probe"
	fenceNamespace = "fence.invalid/fence/fence"
)

// recorder reads a connection until it closes. A gorilla connection is dead
// after its first read timeout, so frames cannot be collected by reading with a
// deadline until one expires — the second read would fail whatever the server
// sent.
type recorder struct {
	mu     sync.Mutex
	frames [][]byte
}

func record(
	conn *websocket.Conn,
) *recorder {
	r := &recorder{}
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			r.mu.Lock()
			r.frames = append(r.frames, msg)
			r.mu.Unlock()
		}
	}()
	return r
}

func (r *recorder) snapshot() [][]byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.frames)
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.frames)
}

// namespaces decodes every frame recorded so far. Decoding is strict, so a
// frame that is not a search result fails here rather than being silently
// tolerated.
func namespaces(
	t *testing.T,
	frames [][]byte,
) []string {
	t.Helper()

	out := make([]string, 0, len(frames))
	for _, frame := range frames {
		out = append(out, decodeResult(t, frame).Namespace)
	}
	return out
}

func decodeResult(
	t *testing.T,
	frame []byte,
) dto.SearchResultDTO {
	t.Helper()

	decoder := json.NewDecoder(bytes.NewReader(frame))
	decoder.DisallowUnknownFields()

	var got dto.SearchResultDTO
	require.NoError(t, decoder.Decode(&got), "frame is not a search result: %s", frame)
	return got
}

// results drops the probe and fence frames, leaving what the test pushed.
func results(
	t *testing.T,
	rec *recorder,
) []string {
	t.Helper()

	out := make([]string, 0, rec.count())
	for _, ns := range namespaces(t, rec.snapshot()) {
		if ns == probeNamespace || ns == fenceNamespace {
			continue
		}
		out = append(out, ns)
	}
	return out
}

func discoveryServer(t *testing.T) (*ws.Handler, *httptest.Server) {
	t.Helper()

	h := ws.NewHandler()
	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	r.GET("/v0/search/discover/:job", h.Discovery.Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return h, srv
}

func discovered(
	ns string,
	name string,
) discovery.Result {
	return discovery.Result{
		Arrow: domain.Arrow{
			Namespace: domain.Namespace(ns + "@v1.2.3"),
			ArrowMeta: domain.ArrowMeta{
				Name:        name,
				Description: name + " description",
				Tags:        []string{"browser"},
				Media:       domain.ArrowMedia{Icon: "icon.png", Banner: "banner.png"},
			},
			Targets: map[domain.OS]domain.Target{
				domain.OSLinuxAMD64:  {},
				domain.OSDarwinARM64: {},
			},
		},
		Namespace: domain.Namespace(ns),
		Stars:     42,
		Source:    "github.com",
	}
}

func item(
	jobID string,
	result discovery.Result,
) usecases.StreamItem {
	return usecases.StreamItem{JobID: jobID, Result: result}
}

// subscribe dials a job stream and returns only once the connection is really
// registered. Broadcaster.WaitRegistered closes a channel once per broadcaster,
// on the first client it ever accepts, so it cannot answer for a second
// subscriber; pushing a probe until one arrives can.
func subscribe(
	t *testing.T,
	h *ws.Handler,
	srv *httptest.Server,
	jobID string,
) *recorder {
	t.Helper()

	rec := record(dial(t, srv, "/v0/search/discover/"+jobID))
	probe := item(jobID, discovery.Result{Namespace: domain.Namespace(probeNamespace)})

	require.Eventually(t, func() bool {
		h.PushDiscovery(probe)
		return rec.count() > 0
	}, 2*time.Second, 10*time.Millisecond)

	return rec
}

// fence pushes a marker to jobID and waits for it. Push dispatches to every
// matching client in one pass and a connection preserves order, so once the
// fence has landed every earlier push has been delivered or dropped — never
// merely in flight.
func fence(
	t *testing.T,
	h *ws.Handler,
	rec *recorder,
	jobID string,
) {
	t.Helper()

	h.PushDiscovery(item(jobID, discovered(fenceNamespace, "Fence")))
	require.Eventually(t, func() bool {
		return slices.Contains(namespaces(t, rec.snapshot()), fenceNamespace)
	}, 2*time.Second, 5*time.Millisecond)
}

func TestNewHandler_CreatesDiscoveryBroadcaster(t *testing.T) {
	assert.NotNil(t, ws.NewHandler().Discovery)
}

// TestDiscoveryStream_SubscriberOnlyGetsOwnJob is the whole point of keying the
// stream on :job — two searches running at once must not bleed into each other.
func TestDiscoveryStream_SubscriberOnlyGetsOwnJob(t *testing.T) {
	h, srv := discoveryServer(t)

	recA := subscribe(t, h, srv, "job-a")
	recB := subscribe(t, h, srv, "job-b")

	h.PushDiscovery(item("job-b", discovered("github.com/user/beta", "Beta")))
	h.PushDiscovery(item("job-a", discovered("github.com/user/alpha", "Alpha")))
	h.PushDiscovery(item("job-b", discovered("github.com/user/gamma", "Gamma")))

	fence(t, h, recA, "job-a")
	fence(t, h, recB, "job-b")

	assert.Equal(t, []string{"github.com/user/alpha"}, results(t, recA))
	assert.Equal(t, []string{"github.com/user/beta", "github.com/user/gamma"}, results(t, recB))
}

// TestDiscoveryStream_EveryFrameIsASearchResult is the regression guard on the
// one-type contract: DisallowUnknownFields fails the moment anyone smuggles a
// discriminator, an error frame or a terminal message into the stream.
func TestDiscoveryStream_EveryFrameIsASearchResult(t *testing.T) {
	h, srv := discoveryServer(t)
	rec := subscribe(t, h, srv, "job-a")

	h.PushDiscovery(item("job-a", discovered("github.com/user/alpha", "Alpha")))
	h.PushDiscovery(item("job-a", discovered("github.com/user/beta", "Beta")))
	fence(t, h, rec, "job-a")

	frames := rec.snapshot()
	require.NotEmpty(t, frames)

	seen := 0
	for _, frame := range frames {
		got := decodeResult(t, frame)
		if got.Namespace == probeNamespace {
			continue
		}
		seen++
		assert.Equal(t, models.ProvenanceSeen, got.Provenance)
		assert.False(t, got.Installed)
	}
	assert.Equal(t, 3, seen, "two results plus the fence")
}

// TestDiscoveryStream_SerializeMatchesLaneADTO proves the client renders one
// shape: a streamed result is identical to what GET /v0/search would emit for
// the same arrow.
func TestDiscoveryStream_SerializeMatchesLaneADTO(t *testing.T) {
	h, srv := discoveryServer(t)
	rec := subscribe(t, h, srv, "job-a")

	h.PushDiscovery(item("job-a", discovered("github.com/user/alpha", "Alpha")))
	fence(t, h, rec, "job-a")

	laneA, err := json.Marshal(dto.SearchResultDTOFrom(models.SearchResult{
		Namespace:    "github.com/user/alpha",
		Name:         "Alpha",
		Description:  "Alpha description",
		Tags:         []string{"browser"},
		Media:        domain.ArrowMedia{Icon: "icon.png", Banner: "banner.png"},
		Versions:     []string{"v1.2.3"},
		CompatibleOS: []domain.OS{domain.OSDarwinARM64, domain.OSLinuxAMD64},
		Provenance:   models.ProvenanceSeen,
		Stars:        42,
		Source:       "github.com",
	}))
	require.NoError(t, err)

	streamed := frameFor(t, rec, "github.com/user/alpha")
	assert.JSONEq(t, string(laneA), string(streamed))
}

func frameFor(
	t *testing.T,
	rec *recorder,
	ns string,
) []byte {
	t.Helper()

	for _, frame := range rec.snapshot() {
		if decodeResult(t, frame).Namespace == ns {
			return frame
		}
	}
	t.Fatalf("no frame for %s", ns)
	return nil
}

func TestDiscoveryStream_UnkeyedSubscriberSeesEveryJob(t *testing.T) {
	h := ws.NewHandler()
	r := gin.New()
	r.GET("/v0/search/discover", h.Discovery.Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	rec := record(dial(t, srv, "/v0/search/discover"))
	h.Discovery.WaitRegistered()

	h.PushDiscovery(item("job-a", discovered("github.com/user/alpha", "Alpha")))
	h.PushDiscovery(item("job-b", discovered("github.com/user/beta", "Beta")))
	fence(t, h, rec, "job-a")

	assert.Equal(t, []string{"github.com/user/alpha", "github.com/user/beta"}, results(t, rec))
}
