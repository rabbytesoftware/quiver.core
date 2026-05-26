package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api"
	apiv0 "github.com/rabbytesoftware/quiver.core/internal/api/v0"
	"github.com/rabbytesoftware/quiver.core/internal/app"
)

func newTestContainer(t *testing.T) *api.Container {
	t.Helper()
	v0, err := apiv0.New(&app.Container{})
	require.NoError(t, err)
	c, err := api.New(api.NewHub(), api.BuildInfo{Version: "0.0.0", BuildID: "0"}, v0)
	require.NoError(t, err)
	return c
}

func TestAPINew_NoVersions_ReturnsError(t *testing.T) {
	hub := api.NewHub()
	c, err := api.New(hub, api.BuildInfo{})
	require.Error(t, err)
	assert.Nil(t, c)
}

func TestAPINew_ValidContainer_ReturnsContainer(t *testing.T) {
	c := newTestContainer(t)
	assert.NotNil(t, c)
}

func TestAPIContainer_ServeHTTP_UnknownRoute_Returns404(t *testing.T) {
	c := newTestContainer(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/no-such-route", nil)
	c.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAPIContainer_ServeHTTP_VersionsRoute_Returns200(t *testing.T) {
	c := newTestContainer(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/versions", nil)
	c.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Version string `json:"version"`
			BuildID string `json:"build_id"`
			API     struct {
				Supported []string `json:"supported"`
				Latest    string   `json:"latest"`
			} `json:"api"`
		} `json:"data"`
	}

	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.Equal(t, "0.0.0", resp.Data.Version)
	assert.Equal(t, "0", resp.Data.BuildID)
	assert.Equal(t, []string{"v0"}, resp.Data.API.Supported)
	assert.Equal(t, "v0", resp.Data.API.Latest)
}

func TestAPIContainer_Run_ServesAndShutdown(t *testing.T) {
	c := newTestContainer(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- c.Run(ln) }()

	resp, err := http.Get("http://" + ln.Addr().String() + "/no-such-route")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, c.Shutdown(ctx))

	err = <-done
	assert.True(t, errors.Is(err, http.ErrServerClosed))
}
