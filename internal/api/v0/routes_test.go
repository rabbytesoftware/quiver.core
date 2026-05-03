package v0

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/api/mocks"
	"github.com/rabbytesoftware/quiver/internal/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainer_Register_MountsHealthRoute(t *testing.T) {
	appContainer := &app.Container{
		Arrow:   &mocks.ArrowService{},
		Runtime: &mocks.RuntimeService{},
		Quiver:  &mocks.QuiverService{},
	}
	c, err := New(appContainer)
	require.NoError(t, err)

	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	c.Register(r.Group(""))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestContainer_Register_MountsArrowRoutes(t *testing.T) {
	appContainer := &app.Container{
		Arrow:   &mocks.ArrowService{},
		Runtime: &mocks.RuntimeService{},
		Quiver:  &mocks.QuiverService{},
	}
	c, err := New(appContainer)
	require.NoError(t, err)

	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	c.Register(r.Group(""))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/arrow", nil))
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestContainer_Register_MountsQuiverRoutes(t *testing.T) {
	appContainer := &app.Container{
		Arrow:   &mocks.ArrowService{},
		Runtime: &mocks.RuntimeService{},
		Quiver:  &mocks.QuiverService{},
	}
	c, err := New(appContainer)
	require.NoError(t, err)

	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	c.Register(r.Group(""))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/quiver", nil))
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}
