package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDaemonClient_TCPScheme(t *testing.T) {
	client, err := newDaemonClient("tcp://127.0.0.1:40257")
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:40257", client.baseURL)
}

func TestNewDaemonClient_UnixScheme(t *testing.T) {
	client, err := newDaemonClient("unix:///custom/quiver.sock")
	require.NoError(t, err)
	assert.Equal(t, "http://unix", client.baseURL)
}

func TestNewDaemonClient_InvalidURI_ReturnsError(t *testing.T) {
	_, err := newDaemonClient("not-a-uri")
	assert.Error(t, err)
}

func TestNewDaemonClient_UnsupportedScheme_ReturnsError(t *testing.T) {
	_, err := newDaemonClient("grpc://localhost:1234")
	assert.Error(t, err)
}

func TestDaemonClient_Do_TCP_SendsRequestAndDecodesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v0/auth/pairing", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]string{"code": "482913"}})
	}))
	defer srv.Close()

	client, err := newDaemonClient("tcp://" + srv.Listener.Addr().String())
	require.NoError(t, err)

	var resp apiResponse[pairingCodeDTO]
	require.NoError(t, client.do(context.Background(), http.MethodPost, "/v0/auth/pairing", nil, &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, "482913", resp.Data.Code)
}

func TestDaemonClient_Do_WithBody_EncodesJSON(t *testing.T) {
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	}))
	defer srv.Close()

	client, err := newDaemonClient("tcp://" + srv.Listener.Addr().String())
	require.NoError(t, err)

	var resp apiResponse[struct{}]
	require.NoError(t, client.do(context.Background(), http.MethodPost, "/v0/auth/pairing/redeem",
		map[string]string{"code": "482913"}, &resp))
	assert.Equal(t, "482913", gotBody["code"])
}

func TestDaemonClient_Do_Unix_DialsTheSocket(t *testing.T) {
	// A short path in the system temp dir, not t.TempDir(): sockaddr_un.sun_path
	// is capped at 104 bytes on macOS, and a per-test t.TempDir() path easily
	// exceeds that.
	f, err := os.CreateTemp("", "qv-*.sock")
	require.NoError(t, err)
	sockPath := f.Name()
	require.NoError(t, f.Close())
	require.NoError(t, os.Remove(sockPath))
	t.Cleanup(func() { _ = os.Remove(sockPath) })

	ln, err := net.Listen("unix", sockPath)
	require.NoError(t, err)
	defer ln.Close()

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v0/auth/devices", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []deviceDTO{{ID: "dev-1"}}})
	})}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	client := newUnixDaemonClient(sockPath)

	var resp apiResponse[[]deviceDTO]
	require.NoError(t, client.do(context.Background(), http.MethodGet, "/v0/auth/devices", nil, &resp))
	assert.True(t, resp.Success)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "dev-1", resp.Data[0].ID)
}

func TestDaemonClient_Do_UnmarshalableBody_ReturnsError(t *testing.T) {
	client, err := newDaemonClient("tcp://127.0.0.1:40257")
	require.NoError(t, err)

	var resp apiResponse[struct{}]
	err = client.do(context.Background(), http.MethodPost, "/v0/auth/pairing/redeem", make(chan int), &resp)
	assert.Error(t, err)
}

func TestDaemonClient_Do_MalformedPath_ReturnsError(t *testing.T) {
	client, err := newDaemonClient("tcp://127.0.0.1:40257")
	require.NoError(t, err)

	var resp apiResponse[struct{}]
	// A control character makes the URL unparseable by http.NewRequestWithContext.
	err = client.do(context.Background(), http.MethodGet, "/v0/auth/\x7f", nil, &resp)
	assert.Error(t, err)
}

func TestDaemonClient_Do_RequestFails_ReturnsError(t *testing.T) {
	client, err := newDaemonClient("tcp://127.0.0.1:1")
	require.NoError(t, err)

	var resp apiResponse[struct{}]
	err = client.do(context.Background(), http.MethodGet, "/v0/auth/devices", nil, &resp)
	assert.Error(t, err)
}

func TestDaemonClient_Do_MalformedResponse_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	client, err := newDaemonClient("tcp://" + srv.Listener.Addr().String())
	require.NoError(t, err)

	var resp apiResponse[struct{}]
	err = client.do(context.Background(), http.MethodGet, "/v0/auth/devices", nil, &resp)
	assert.Error(t, err)
}

func TestNewDaemonClient_EmptyHost_ReadsConfig(t *testing.T) {
	client, err := newDaemonClient("")
	require.NoError(t, err)
	assert.NotNil(t, client)
}
