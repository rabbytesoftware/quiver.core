package home_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/mocks"
	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	home "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/home/handlers"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func serve(
	svc usecases.HomeUsecase,
	method string,
	target string,
) *httptest.ResponseRecorder {
	h := home.New(svc)
	r := gin.New()
	r.GET("/v0/home", h.Home)
	r.POST("/v0/home/refresh", h.Refresh)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, target, nil))
	return w
}

func TestHomeHandler_ReturnsTheShelvesWrappedLikeEveryQuery(t *testing.T) {
	svc := &mocks.HomeService{HomeResult: models.Home{
		Refreshing: true,
		Shelves: []models.HomeShelf{{
			ID:          "popular",
			Title:       "Popular",
			RefreshedAt: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC),
			Arrows:      []models.SearchResult{{Namespace: domain.Namespace("github.com/a/one"), Name: "One"}},
		}},
	}}

	w := serve(svc, http.MethodGet, "/v0/home")

	require.Equal(t, http.StatusOK, w.Code)
	var env struct {
		Success bool           `json:"success"`
		Error   *string        `json:"error"`
		Data    apidto.HomeDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.True(t, env.Success)
	assert.Nil(t, env.Error)
	assert.True(t, env.Data.Refreshing)
	require.Len(t, env.Data.Shelves, 1)
	assert.Equal(t, "popular", env.Data.Shelves[0].ID)
	require.Len(t, env.Data.Shelves[0].Arrows, 1)
	assert.Equal(t, "github.com/a/one", env.Data.Shelves[0].Arrows[0].Namespace)
	assert.Equal(t, 1, svc.HomeCalls)
}

func TestHomeHandler_ServiceError_MapsToItsStatus(t *testing.T) {
	svc := &mocks.HomeService{HomeErr: apperrors.ErrNotFound}

	w := serve(svc, http.MethodGet, "/v0/home")

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHomeHandler_UnclassifiedError_Is500(t *testing.T) {
	svc := &mocks.HomeService{HomeErr: errors.New("boom")}

	w := serve(svc, http.MethodGet, "/v0/home")

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHomeHandler_NoService_Is503(t *testing.T) {
	w := serve(nil, http.MethodGet, "/v0/home")

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestRefreshHandler_TriggersARefreshAndReturns202WithNoBody(t *testing.T) {
	svc := &mocks.HomeService{}

	w := serve(svc, http.MethodPost, "/v0/home/refresh")

	assert.Equal(t, http.StatusAccepted, w.Code)
	assert.Empty(t, w.Body.Bytes())
	assert.Equal(t, 1, svc.RefreshCalls)
}

func TestRefreshHandler_NoService_Is503(t *testing.T) {
	w := serve(nil, http.MethodPost, "/v0/home/refresh")

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
