package ws_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ws "github.com/rabbytesoftware/quiver.core/internal/api/ws"
)

type evt struct {
	Key  string `json:"key"`
	Seq  uint64 `json:"seq"`
	Name string `json:"name"`
}

type journal struct {
	mu    sync.Mutex
	items []evt
	done  chan struct{}
}

func newJournal() *journal {
	return &journal{done: make(chan struct{})}
}

func (j *journal) add(e evt) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.items = append(j.items, e)
}

func (j *journal) replay(string) []evt {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]evt(nil), j.items...)
}

func streamServer(
	t *testing.T,
	j *journal,
	policy ws.OverflowPolicy,
) (*ws.Broadcaster[evt], *httptest.Server) {
	t.Helper()
	b := ws.NewBroadcaster(ws.StreamDef[evt]{
		KeyParam:  "job",
		KeyMatch:  ws.ExactMatch,
		Overflow:  policy,
		Namespace: func(e evt) string { return e.Key },
		Serialize: func(e evt) ([]byte, error) { return json.Marshal(e) },
		Seq:       func(e evt) uint64 { return e.Seq },
		Replay:    j.replay,
		Done:      func(string) <-chan struct{} { return j.done },
		Filters: []ws.FilterDef[evt]{
			{Param: "name", Extract: func(e evt) string { return e.Name }, Match: ws.ExactMatch},
		},
	})
	r := gin.New()
	r.GET("/jobs/:job", b.Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return b, srv
}

func readSeqs(
	t *testing.T,
	conn *websocket.Conn,
) ([]uint64, error) {
	t.Helper()
	var seqs []uint64
	for {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return seqs, err
		}
		var e evt
		require.NoError(t, json.Unmarshal(msg, &e))
		seqs = append(seqs, e.Seq)
	}
}

func TestBroadcaster_Handle_ReplaysAndClosesNormallyWhenDone(t *testing.T) {
	j := newJournal()
	j.add(evt{Key: "j1", Seq: 1, Name: "a"})
	j.add(evt{Key: "j1", Seq: 2, Name: "b"})
	close(j.done)
	_, srv := streamServer(t, j, ws.OverflowDisconnect)

	conn := wsDial(t, srv, "/jobs/j1")
	seqs, err := readSeqs(t, conn)

	assert.Equal(t, []uint64{1, 2}, seqs)
	assert.True(t, websocket.IsCloseError(err, websocket.CloseNormalClosure), "got %v", err)
}

func TestBroadcaster_Handle_ReplayHonoursPredicate(t *testing.T) {
	j := newJournal()
	j.add(evt{Key: "j1", Seq: 1, Name: "a"})
	j.add(evt{Key: "j1", Seq: 2, Name: "b"})
	close(j.done)
	_, srv := streamServer(t, j, ws.OverflowDisconnect)

	conn := wsDial(t, srv, "/jobs/j1?name=b")
	seqs, _ := readSeqs(t, conn)

	assert.Equal(t, []uint64{2}, seqs)
}

func TestBroadcaster_Handle_ReplayLongerThanBufferIsDeliveredWhole(t *testing.T) {
	j := newJournal()
	const total = 300
	for i := uint64(1); i <= total; i++ {
		j.add(evt{Key: "j1", Seq: i})
	}
	close(j.done)
	_, srv := streamServer(t, j, ws.OverflowDisconnect)

	conn := wsDial(t, srv, "/jobs/j1")
	seqs, err := readSeqs(t, conn)

	assert.Len(t, seqs, total)
	assert.True(t, websocket.IsCloseError(err, websocket.CloseNormalClosure))
}

func TestBroadcaster_Handle_LiveFrameAlreadyReplayedIsNotDuplicated(t *testing.T) {
	j := newJournal()
	j.add(evt{Key: "j1", Seq: 1, Name: "a"})
	b, srv := streamServer(t, j, ws.OverflowDisconnect)

	conn := wsDial(t, srv, "/jobs/j1")
	b.WaitRegistered()
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(msg), `"seq":1`)

	b.Push(evt{Key: "j1", Seq: 1, Name: "a"})
	j.add(evt{Key: "j1", Seq: 2, Name: "b"})
	b.Push(evt{Key: "j1", Seq: 2, Name: "b"})
	close(j.done)

	seqs, err := readSeqs(t, conn)
	assert.Equal(t, []uint64{2}, seqs)
	assert.True(t, websocket.IsCloseError(err, websocket.CloseNormalClosure))
}

func TestBroadcaster_Handle_LiveFramesFlushBeforeTerminalClose(t *testing.T) {
	j := newJournal()
	b, srv := streamServer(t, j, ws.OverflowDisconnect)

	conn := wsDial(t, srv, "/jobs/j1")
	b.WaitRegistered()
	b.Push(evt{Key: "j1", Seq: 1})
	close(j.done)

	seqs, err := readSeqs(t, conn)
	assert.Equal(t, []uint64{1}, seqs)
	assert.True(t, websocket.IsCloseError(err, websocket.CloseNormalClosure))
}

func TestBroadcaster_Push_OverflowDisconnectClosesWithTryAgainLater(t *testing.T) {
	j := newJournal()
	b, srv := streamServer(t, j, ws.OverflowDisconnect)

	conn := wsDial(t, srv, "/jobs/j1")
	b.WaitRegistered()
	heavy := strings.Repeat("x", 256*1024)
	for i := uint64(1); i <= 400; i++ {
		b.Push(evt{Key: "j1", Seq: i, Name: heavy})
	}

	_, err := readSeqs(t, conn)
	assert.True(t, websocket.IsCloseError(err, websocket.CloseTryAgainLater), "got %v", err)
}

func TestBroadcaster_Push_OverflowDropKeepsConnectionOpen(t *testing.T) {
	b, srv := setupBroadcaster(t)
	conn := wsDial(t, srv, "/items")
	b.WaitRegistered()
	heavy := strings.Repeat("x", 256*1024)
	for i := 0; i < 400; i++ {
		b.Push(item{Name: heavy})
	}

	var got item
	wsRead(t, conn, &got)
	assert.Len(t, got.Name, len(heavy))
}

func TestBroadcaster_Handle_LiveFramesDuringReplayAreHeldThenStitched(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	j := newJournal()
	j.add(evt{Key: "j1", Seq: 1})
	b := ws.NewBroadcaster(ws.StreamDef[evt]{
		KeyParam:  "job",
		KeyMatch:  ws.ExactMatch,
		Namespace: func(e evt) string { return e.Key },
		Serialize: func(e evt) ([]byte, error) { return json.Marshal(e) },
		Seq:       func(e evt) uint64 { return e.Seq },
		Replay: func(key string) []evt {
			close(entered)
			<-release
			return j.replay(key)
		},
		Done: func(string) <-chan struct{} { return j.done },
	})
	r := gin.New()
	r.GET("/jobs/:job", b.Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := wsDial(t, srv, "/jobs/j1")
	<-entered
	b.Push(evt{Key: "j1", Seq: 1})
	b.Push(evt{Key: "j1", Seq: 2})
	j.add(evt{Key: "j1", Seq: 2})
	close(release)
	close(j.done)

	seqs, err := readSeqs(t, conn)
	assert.Equal(t, []uint64{1, 2}, seqs)
	assert.True(t, websocket.IsCloseError(err, websocket.CloseNormalClosure))
}
