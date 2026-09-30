package ws_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/api/mocks"
	ws "github.com/rabbytesoftware/quiver.core/internal/api/v0/ws"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func jobServer(
	t *testing.T,
	jobs usecases.DiscoveryUsecase,
) (*ws.Handler, *httptest.Server) {
	t.Helper()

	h := ws.NewHandler(ws.WithDiscoveryJobs(jobs))
	r := gin.New()
	r.GET("/v0/search/discover/:job", h.Discovery.Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return h, srv
}

func seqItem(
	seq uint64,
	ns string,
	name string,
) usecases.StreamItem {
	return usecases.StreamItem{JobID: "job-1", Seq: seq, Result: discovered(ns, name)}
}

func readNamespaces(
	t *testing.T,
	conn *websocket.Conn,
) ([]string, error) {
	t.Helper()

	var out []string
	for {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return out, err
		}
		out = append(out, decodeResult(t, msg).Namespace)
	}
}

func TestDiscoveryStream_LateSubscriberIsReplayedThenClosedWhenJobIsDone(t *testing.T) {
	jobs := &mocks.DiscoveryService{}
	jobs.SetReplay([]usecases.StreamItem{
		seqItem(1, "github.com/u/alpha", "Alpha"),
		seqItem(2, "github.com/u/beta", "Beta"),
	})
	jobs.Finish()
	_, srv := jobServer(t, jobs)

	got, err := readNamespaces(t, dial(t, srv, "/v0/search/discover/job-1"))

	assert.Equal(t, []string{"github.com/u/alpha", "github.com/u/beta"}, got)
	assert.True(t, websocket.IsCloseError(err, websocket.CloseNormalClosure), "got %v", err)
}

func TestDiscoveryStream_LiveResultsFollowReplayWithoutDuplicates(t *testing.T) {
	jobs := &mocks.DiscoveryService{}
	jobs.SetReplay([]usecases.StreamItem{seqItem(1, "github.com/u/alpha", "Alpha")})
	h, srv := jobServer(t, jobs)

	conn := dial(t, srv, "/v0/search/discover/job-1")
	h.Discovery.WaitRegistered()

	h.PushDiscovery(seqItem(1, "github.com/u/alpha", "Alpha"))
	h.PushDiscovery(seqItem(2, "github.com/u/beta", "Beta"))
	jobs.Finish()

	got, err := readNamespaces(t, conn)
	assert.Equal(t, []string{"github.com/u/alpha", "github.com/u/beta"}, got)
	assert.True(t, websocket.IsCloseError(err, websocket.CloseNormalClosure), "got %v", err)
}

func TestDiscoveryStream_OSFilterAppliesToReplayAndLive(t *testing.T) {
	linuxOnly := discovered("github.com/u/linux", "Linux")
	linuxOnly.Arrow.Targets = map[domain.OS]domain.Target{domain.OSLinuxAMD64: {}}
	windowsOnly := discovered("github.com/u/win", "Win")
	windowsOnly.Arrow.Targets = map[domain.OS]domain.Target{domain.OSWindowsAMD64: {}}

	testCases := []struct {
		name string
		path string
		want []string
	}{
		{"matching platform", "?os=windows/amd64", []string{"github.com/u/win"}},
		{"unknown platform selects nothing", "?os=plan9/386", nil},
		{"no filter selects all", "", []string{"github.com/u/linux", "github.com/u/win"}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jobs := &mocks.DiscoveryService{}
			jobs.SetReplay([]usecases.StreamItem{{JobID: "job-1", Seq: 1, Result: linuxOnly}})
			h, srv := jobServer(t, jobs)

			conn := dial(t, srv, "/v0/search/discover/job-1"+tc.path)
			h.Discovery.WaitRegistered()
			h.PushDiscovery(usecases.StreamItem{JobID: "job-1", Seq: 2, Result: windowsOnly})
			jobs.Finish()

			got, _ := readNamespaces(t, conn)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDiscoveryStream_WithoutJobsIsLiveOnlyAndNeverCloses(t *testing.T) {
	h, srv := discoveryServer(t)

	rec := subscribe(t, h, srv, "job-1")
	h.PushDiscovery(item("job-1", discovery.Result{Namespace: domain.Namespace("github.com/u/a")}))
	fence(t, h, rec, "job-1")

	assert.Equal(t, []string{"github.com/u/a"}, results(t, rec))
}
