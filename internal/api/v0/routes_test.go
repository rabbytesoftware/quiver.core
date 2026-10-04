package v0

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/app"
)

func TestContainer_Register_MountsHealthRoute(t *testing.T) {
	appContainer := &app.Container{
		Arrow:      &mocks.ArrowService{},
		Runtime:    &mocks.RuntimeService{},
		Collection: &mocks.CollectionService{},
	}
	c, err := New(appContainer, nil, "")
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
		Arrow:      &mocks.ArrowService{},
		Runtime:    &mocks.RuntimeService{},
		Collection: &mocks.CollectionService{},
	}
	c, err := New(appContainer, nil, "")
	require.NoError(t, err)

	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	c.Register(r.Group(""))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/arrow", nil))
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestContainer_Register_MountsSearchRoute(t *testing.T) {
	appContainer := &app.Container{
		Arrow:      &mocks.ArrowService{},
		Runtime:    &mocks.RuntimeService{},
		Collection: &mocks.CollectionService{},
		Search:     &mocks.SearchService{},
	}
	c, err := New(appContainer, nil, "")
	require.NoError(t, err)

	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	c.Register(r.Group(""))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/search?q=redis", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestContainer_Register_MountsQuiverRoutes(t *testing.T) {
	appContainer := &app.Container{
		Arrow:      &mocks.ArrowService{},
		Runtime:    &mocks.RuntimeService{},
		Collection: &mocks.CollectionService{},
	}
	c, err := New(appContainer, nil, "")
	require.NoError(t, err)

	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	c.Register(r.Group(""))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/collection", nil))
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestContainer_Register_MountsHomeRoutes(t *testing.T) {
	home := &mocks.HomeService{}
	c, err := New(&app.Container{
		Arrow:      &mocks.ArrowService{},
		Runtime:    &mocks.RuntimeService{},
		Collection: &mocks.CollectionService{},
		Home:       home,
	}, nil, "")
	require.NoError(t, err)

	r := gin.New()
	c.Register(r.Group(""))

	read := httptest.NewRecorder()
	r.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/home", nil))
	refresh := httptest.NewRecorder()
	r.ServeHTTP(refresh, httptest.NewRequest(http.MethodPost, "/home/refresh", nil))

	assert.Equal(t, http.StatusOK, read.Code)
	assert.Equal(t, http.StatusAccepted, refresh.Code)
}

func TestContainer_Register_WithoutHome_RoutesAnswer503(t *testing.T) {
	c, err := New(&app.Container{
		Arrow:      &mocks.ArrowService{},
		Runtime:    &mocks.RuntimeService{},
		Collection: &mocks.CollectionService{},
	}, nil, "")
	require.NoError(t, err)

	r := gin.New()
	c.Register(r.Group(""))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/home", nil))

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
