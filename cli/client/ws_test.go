package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var wsUpgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

// wsTestServer starts a WS server that sends msgs then waits for the client to disconnect.
func wsTestServer(t *testing.T, msgs []ArrowRuntime) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()
		for _, m := range msgs {
			data, err := json.Marshal(m)
			require.NoError(t, err)
			require.NoError(t, conn.WriteMessage(websocket.TextMessage, data))
		}
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func TestPump_TerminatesOnTerminalState(t *testing.T) {
	msgs := []ArrowRuntime{
		{Namespace: "github.com/foo/bar", State: "installing"},
		{Namespace: "github.com/foo/bar", State: "ready"},
		{Namespace: "github.com/foo/bar", State: "ready"},
	}
	url := wsTestServer(t, msgs)

	ch, err := pump(context.Background(), url, terminalInstall)
	require.NoError(t, err)

	var got []ArrowRuntime
	for rt := range ch {
		got = append(got, rt)
	}

	require.Len(t, got, 2)
	assert.Equal(t, "installing", got[0].State)
	assert.Equal(t, "ready", got[1].State)
}

func TestPump_TerminatesOnCtxCancel(t *testing.T) {
	url := wsTestServer(t, []ArrowRuntime{
		{Namespace: "github.com/foo/bar", State: "installing"},
	})

	ctx, cancel := context.WithCancel(context.Background())

	ch, err := pump(ctx, url, neverStop)
	require.NoError(t, err)

	rt, ok := <-ch
	require.True(t, ok)
	assert.Equal(t, "installing", rt.State)

	cancel()

	_, ok = <-ch
	assert.False(t, ok, "channel must close after ctx cancellation")
}

func TestPump_TerminalCustomMethod(t *testing.T) {
	activeRun := &RunRecord{Method: "deploy"}
	msgs := []ArrowRuntime{
		{Namespace: "ns", State: "running", ActiveRun: activeRun},
		{Namespace: "ns", State: "ready"},
	}
	url := wsTestServer(t, msgs)

	ch, err := pump(context.Background(), url, terminalCustomMethod)
	require.NoError(t, err)

	var got []ArrowRuntime
	for rt := range ch {
		got = append(got, rt)
	}

	require.Len(t, got, 2)
	assert.NotNil(t, got[0].ActiveRun)
	assert.Nil(t, got[1].ActiveRun)
}

func TestPump_ConnectionError_ReturnsErr(t *testing.T) {
	_, err := pump(context.Background(), "ws://127.0.0.1:1", neverStop)
	assert.Error(t, err)
}
