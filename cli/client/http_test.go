package client_test

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

	"quiver-cli/client"
)

// apiOK writes a standard success envelope around data.
func apiOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
}

// apiErr writes a standard error envelope.
func apiErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"success": false, "error": msg})
}

// httpClient creates a test HTTP server with the given handler and returns an HTTPClient pointing at it.
func httpClient(t *testing.T, handler http.HandlerFunc) *client.HTTPClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return client.NewHTTPClient(srv.URL)
}

// --- ArrowList ---

func TestHTTPClient_ArrowList_ReturnsItems(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v0/arrow", r.URL.Path)
		assert.Equal(t, "true", r.URL.Query().Get("user_installed"))
		apiOK(w, []client.ArrowListItem{
			{Namespace: "github.com/foo/bar", Name: "bar"},
		})
	})

	items, err := c.ArrowList(context.Background(), true)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "github.com/foo/bar", items[0].Namespace)
}

func TestHTTPClient_ArrowList_ServerError_ReturnsErr(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		apiErr(w, http.StatusInternalServerError, "internal error")
	})
	_, err := c.ArrowList(context.Background(), false)
	assert.Error(t, err)
}

// --- ArrowGet ---

func TestHTTPClient_ArrowGet_ReturnsDetail(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v0/arrow/github.com%2Ffoo%2Fbar", r.URL.RawPath)
		apiOK(w, client.ArrowDetail{Namespace: "github.com/foo/bar", State: "ready"})
	})

	detail, err := c.ArrowGet(context.Background(), "github.com/foo/bar")
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.Equal(t, "ready", detail.State)
}

func TestHTTPClient_ArrowGet_NotFound_ReturnsErr(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		apiErr(w, http.StatusNotFound, "not found")
	})
	_, err := c.ArrowGet(context.Background(), "github.com/foo/bar")
	assert.Error(t, err)
}

// --- ArrowGetManifest ---

func TestHTTPClient_ArrowGetManifest_ReturnsDataBytes(t *testing.T) {
	manifest := map[string]any{"name": "bar", "version": "1.0.0"}
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v0/arrow/github.com%2Ffoo%2Fbar/manifest", r.URL.RawPath)
		apiOK(w, manifest)
	})

	data, err := c.ArrowGetManifest(context.Background(), "github.com/foo/bar")
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "bar", decoded["name"])
}

// --- ArrowAdd ---

func TestHTTPClient_ArrowAdd_Success(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v0/arrow/github.com%2Ffoo%2Fbar", r.URL.RawPath)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"success": true, "namespace": "github.com/foo/bar"})
	})
	assert.NoError(t, c.ArrowAdd(context.Background(), "github.com/foo/bar"))
}

func TestHTTPClient_ArrowAdd_Conflict_ReturnsErr(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		apiErr(w, http.StatusConflict, "already registered")
	})
	assert.Error(t, c.ArrowAdd(context.Background(), "github.com/foo/bar"))
}

// --- ArrowUpdate ---

func TestHTTPClient_ArrowUpdate_Success(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		json.NewEncoder(w).Encode(map[string]any{"success": true})
	})
	assert.NoError(t, c.ArrowUpdate(context.Background(), "github.com/foo/bar"))
}

// --- ArrowRemove ---

func TestHTTPClient_ArrowRemove_Success(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/v0/arrow/github.com%2Ffoo%2Fbar%40v1.0.0", r.URL.RawPath)
		json.NewEncoder(w).Encode(map[string]any{"success": true})
	})
	assert.NoError(t, c.ArrowRemove(context.Background(), "github.com/foo/bar@v1.0.0"))
}

// --- ArrowSeed ---

func TestHTTPClient_ArrowSeed_SendsYAMLBody(t *testing.T) {
	manifest := []byte("name: bar\nversion: 1.0.0")
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v0/arrow/github.com%2Ffoo%2Fbar/manifest", r.URL.RawPath)
		assert.Equal(t, "application/x-yaml", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"success": true})
	})
	assert.NoError(t, c.ArrowSeed(context.Background(), "github.com/foo/bar", manifest))
}

// --- ArrowValidate ---

func TestHTTPClient_ArrowValidate_Valid(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v0/arrow/github.com%2Ffoo%2Fbar/manifest/validate", r.URL.RawPath)
		apiOK(w, client.ValidationResult{Valid: true, SupportedPlatforms: []string{"linux"}})
	})

	result, err := c.ArrowValidate(context.Background(), "github.com/foo/bar", []byte("name: bar"))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Valid)
}

func TestHTTPClient_ArrowValidate_Invalid_ReturnsResult(t *testing.T) {
	c := httpClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Server returns 422 for invalid but same body shape.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"data": client.ValidationResult{
				Valid:  false,
				Errors: []client.ValidationError{{Field: "name", Rule: "required", Message: "name is required"}},
			},
		})
	})

	result, err := c.ArrowValidate(context.Background(), "github.com/foo/bar", []byte("version: 1.0.0"))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Valid)
	require.Len(t, result.Errors, 1)
	assert.Equal(t, "name", result.Errors[0].Field)
}

// --- Runtime tests ---
// These tests use a combined HTTP+WS server: the POST handler returns 202,
// then the WS handler streams pre-canned ArrowRuntime snapshots.

var testWSUpgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

// lifecycleServer creates a server that accepts POST (returns 202) and WS (streams snapshots).
func lifecycleServer(t *testing.T, snapshots []client.ArrowRuntime) *client.HTTPClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			conn, err := testWSUpgrader.Upgrade(w, r, nil)
			require.NoError(t, err)
			defer conn.Close()
			for _, s := range snapshots {
				data, err := json.Marshal(s)
				require.NoError(t, err)
				require.NoError(t, conn.WriteMessage(websocket.TextMessage, data))
			}
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		} else {
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]any{"success": true})
		}
	}))
	t.Cleanup(srv.Close)
	return client.NewHTTPClient(srv.URL)
}

func TestHTTPClient_Install_StreamsSnapshots(t *testing.T) {
	snapshots := []client.ArrowRuntime{
		{Namespace: "github.com/foo/bar", State: "installing"},
		{Namespace: "github.com/foo/bar", State: "ready"},
	}
	c := lifecycleServer(t, snapshots)

	ch, err := c.Install(context.Background(), "github.com/foo/bar", nil)
	require.NoError(t, err)

	var got []client.ArrowRuntime
	for rt := range ch {
		got = append(got, rt)
	}
	require.Len(t, got, 2)
	assert.Equal(t, "installing", got[0].State)
	assert.Equal(t, "ready", got[1].State)
}

func TestHTTPClient_Uninstall_ClosesOnRemoved(t *testing.T) {
	snapshots := []client.ArrowRuntime{
		{Namespace: "github.com/foo/bar", State: "uninstalling"},
		{Namespace: "github.com/foo/bar", State: "removed"},
	}
	c := lifecycleServer(t, snapshots)

	ch, err := c.Uninstall(context.Background(), "github.com/foo/bar", nil)
	require.NoError(t, err)

	var got []client.ArrowRuntime
	for rt := range ch {
		got = append(got, rt)
	}
	require.Len(t, got, 2)
	assert.Equal(t, "removed", got[1].State)
}

func TestHTTPClient_Run_ClosesOnReady(t *testing.T) {
	snapshots := []client.ArrowRuntime{
		{Namespace: "ns", State: "running"},
		{Namespace: "ns", State: "ready"},
	}
	c := lifecycleServer(t, snapshots)

	ch, err := c.Run(context.Background(), "ns", map[string]string{"key": "val"})
	require.NoError(t, err)

	var got []client.ArrowRuntime
	for rt := range ch {
		got = append(got, rt)
	}
	assert.Equal(t, "ready", got[len(got)-1].State)
}

func TestHTTPClient_Stop_ClosesOnReady(t *testing.T) {
	c := lifecycleServer(t, []client.ArrowRuntime{{Namespace: "ns", State: "ready"}})
	ch, err := c.Stop(context.Background(), "ns")
	require.NoError(t, err)
	var got []client.ArrowRuntime
	for rt := range ch {
		got = append(got, rt)
	}
	require.Len(t, got, 1)
	assert.Equal(t, "ready", got[0].State)
}

func TestHTTPClient_RunMethod_ClosesAfterActiveRunClears(t *testing.T) {
	activeRun := &client.RunRecord{Method: "deploy"}
	snapshots := []client.ArrowRuntime{
		{Namespace: "ns", State: "running", ActiveRun: activeRun},
		{Namespace: "ns", State: "ready"},
	}
	c := lifecycleServer(t, snapshots)

	ch, err := c.RunMethod(context.Background(), "ns", "deploy", nil)
	require.NoError(t, err)
	var got []client.ArrowRuntime
	for rt := range ch {
		got = append(got, rt)
	}
	require.Len(t, got, 2)
	assert.NotNil(t, got[0].ActiveRun)
	assert.Nil(t, got[1].ActiveRun)
}

func TestHTTPClient_WatchRuntime_ClosesOnCtxCancel(t *testing.T) {
	snapshots := []client.ArrowRuntime{{Namespace: "ns", State: "ready"}}
	c := lifecycleServer(t, snapshots)

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := c.WatchRuntime(ctx, "ns")
	require.NoError(t, err)

	rt, ok := <-ch
	require.True(t, ok)
	assert.Equal(t, "ready", rt.State)

	cancel()
	_, ok = <-ch
	assert.False(t, ok)
}

func TestHTTPClient_RuntimeGet_ReturnsSingleSnapshot(t *testing.T) {
	snapshots := []client.ArrowRuntime{{Namespace: "ns", State: "ready"}}
	c := lifecycleServer(t, snapshots)

	rt, err := c.RuntimeGet(context.Background(), "ns")
	require.NoError(t, err)
	require.NotNil(t, rt)
	assert.Equal(t, "ready", rt.State)
}
