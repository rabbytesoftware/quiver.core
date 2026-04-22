package ws_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	ws "github.com/rabbytesoftware/quiver/internal/api/ws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type item struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

var itemDef = ws.StreamDef[item]{
	Namespace: func(i item) string { return i.Name },
	Serialize: func(i item) ([]byte, error) { return json.Marshal(i) },
	Filters: []ws.FilterDef[item]{
		{
			Param:   "kind",
			Extract: func(i item) string { return i.Kind },
			Match:   ws.ExactMatch,
		},
	},
}

func setupBroadcaster(t *testing.T) (*ws.Broadcaster[item], *httptest.Server) {
	t.Helper()
	b := ws.NewBroadcaster(itemDef)
	r := gin.New()
	r.GET("/items", b.Handle)
	r.GET("/items/:ns", b.Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return b, srv
}

func wsDial(t *testing.T, srv *httptest.Server, path string) *websocket.Conn {
	t.Helper()
	url := "ws" + srv.URL[len("http"):] + path
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return conn
}

func wsRead(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(msg, v))
}

func TestBroadcaster_Push_DeliversToClient(t *testing.T) {
	b, srv := setupBroadcaster(t)
	conn := wsDial(t, srv, "/items")
	b.WaitRegistered()

	b.Push(item{Name: "alpha", Kind: "fruit"})

	var got item
	wsRead(t, conn, &got)
	assert.Equal(t, "alpha", got.Name)
}

func TestBroadcaster_Push_FieldFilter(t *testing.T) {
	b, srv := setupBroadcaster(t)
	conn := wsDial(t, srv, "/items?kind=fruit")
	b.WaitRegistered()

	b.Push(item{Name: "carrot", Kind: "vegetable"})
	b.Push(item{Name: "apple", Kind: "fruit"})

	var got item
	wsRead(t, conn, &got)
	assert.Equal(t, "apple", got.Name)
}

func TestBroadcaster_Push_NamespaceGlob(t *testing.T) {
	b, srv := setupBroadcaster(t)
	conn := wsDial(t, srv, "/items/alpha")
	b.WaitRegistered()

	b.Push(item{Name: "beta", Kind: "fruit"})
	b.Push(item{Name: "alpha", Kind: "fruit"})

	var got item
	wsRead(t, conn, &got)
	assert.Equal(t, "alpha", got.Name)
}

func TestBroadcaster_UpgradeRejectsNonWS(t *testing.T) {
	_, srv := setupBroadcaster(t)
	resp, err := http.Get(srv.URL + "/items")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestBroadcaster_ClientDisconnect_ExitsCleanly(t *testing.T) {
	b, srv := setupBroadcaster(t)
	conn := wsDial(t, srv, "/items")
	b.WaitRegistered()

	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	conn.Close()

	b.Push(item{Name: "safe", Kind: "fruit"})
}

func TestBroadcaster_SlowConsumer_DoesNotBlock(t *testing.T) {
	b, srv := setupBroadcaster(t)
	_ = wsDial(t, srv, "/items")
	b.WaitRegistered()

	for i := 0; i < 65; i++ {
		b.Push(item{Name: fmt.Sprintf("item%d", i), Kind: "fruit"})
	}
}

func TestBroadcaster_Push_SerializationError_SkipsDelivery(t *testing.T) {
	errDef := ws.StreamDef[item]{
		Namespace: func(i item) string { return i.Name },
		Serialize: func(i item) ([]byte, error) { return nil, fmt.Errorf("marshal failed") },
		Filters:   []ws.FilterDef[item]{},
	}
	b := ws.NewBroadcaster(errDef)
	r := gin.New()
	r.GET("/items", b.Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := wsDial(t, srv, "/items")
	b.WaitRegistered()

	// Push with serialization error should not crash or deliver
	b.Push(item{Name: "error", Kind: "test"})

	// Set a read deadline to ensure no message arrives
	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, _, err := conn.ReadMessage()
	// Should timeout (no message)
	assert.Error(t, err)
}
