package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver/internal/api"
	apiv0 "github.com/rabbytesoftware/quiver/internal/api/v0"
	"github.com/rabbytesoftware/quiver/internal/app"
)

func TestAPINew_NoVersions_ReturnsError(t *testing.T) {
	hub := api.NewHub()
	c, err := api.New(hub)
	require.Error(t, err)
	assert.Nil(t, c)
}

func TestAPINew_ValidContainer_ReturnsContainer(t *testing.T) {
	v0, err := apiv0.New(&app.Container{})
	require.NoError(t, err)
	c, err := api.New(api.NewHub(), v0)
	require.NoError(t, err)
	assert.NotNil(t, c)
}

func TestAPIContainer_ServeHTTP_UnknownRoute_Returns404(t *testing.T) {
	v0, err := apiv0.New(&app.Container{})
	require.NoError(t, err)
	c, err := api.New(api.NewHub(), v0)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/no-such-route", nil)
	c.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAPIContainer_Run_InvalidAddr_ReturnsError(t *testing.T) {
	v0, err := apiv0.New(&app.Container{})
	require.NoError(t, err)
	c, err := api.New(api.NewHub(), v0)
	require.NoError(t, err)
	err = c.Run("!!!invalid!!!", 1)
	assert.Error(t, err)
}
