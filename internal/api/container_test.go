package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/api"
	"github.com/rabbytesoftware/quiver/internal/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIInit_NilAppContainer_ReturnsError(t *testing.T) {
	hub := api.NewHub()
	c, err := api.Init(nil, hub)
	require.Error(t, err)
	assert.Nil(t, c)
}

func TestAPIInit_ValidContainer_ReturnsContainer(t *testing.T) {
	c, err := api.Init(&app.Container{}, api.NewHub())
	require.NoError(t, err)
	assert.NotNil(t, c)
}

func TestAPIContainer_ServeHTTP_UnknownRoute_Returns404(t *testing.T) {
	c, err := api.Init(&app.Container{}, api.NewHub())
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/no-such-route", nil)
	c.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAPIContainer_Run_InvalidAddr_ReturnsError(t *testing.T) {
	c, err := api.Init(&app.Container{}, api.NewHub())
	require.NoError(t, err)
	err = c.Run("!!!invalid!!!", 0)
	assert.Error(t, err)
}
