package quivers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/api/mocks"
	quivers "github.com/rabbytesoftware/quiver/internal/api/v1/endpoints/quivers/handlers"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	appquiver "github.com/rabbytesoftware/quiver/internal/app/quiver"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

const encodedNS = "/v1/quiver/github.com%2Fuser%2Frepo"

func setup(svc *mocks.QuiverService) *gin.Engine {
	h := quivers.New(svc)
	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	r.POST("/v1/quiver/:ns", h.Add)
	r.PATCH("/v1/quiver/:ns", h.Update)
	r.DELETE("/v1/quiver/:ns", h.Remove)
	r.GET("/v1/quiver", h.List)
	r.GET("/v1/quiver/:ns", h.Get)
	return r
}

func TestQuiverAdd_Created(t *testing.T) {
	r := setup(&mocks.QuiverService{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS, nil))
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestQuiverAdd_Conflict(t *testing.T) {
	r := setup(&mocks.QuiverService{AddErr: apperrors.ErrAlreadyExists})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS, nil))
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestQuiverUpdate_OK(t *testing.T) {
	r := setup(&mocks.QuiverService{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, encodedNS, nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestQuiverUpdate_ServiceError(t *testing.T) {
	r := setup(&mocks.QuiverService{UpdateErr: apperrors.ErrNotFound})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, encodedNS, nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestQuiverRemove_OK(t *testing.T) {
	r := setup(&mocks.QuiverService{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, encodedNS, nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestQuiverRemove_NotFound(t *testing.T) {
	r := setup(&mocks.QuiverService{RemoveErr: apperrors.ErrNotFound})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, encodedNS, nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestQuiverList_OK(t *testing.T) {
	svc := &mocks.QuiverService{
		ListResult: []appquiver.QuiverListDTO{
			{Namespace: domain.Namespace("github.com/user/repo"), Name: "My Quiver"},
		},
	}
	r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/quiver", nil))
	assert.Equal(t, http.StatusOK, w.Code)

	var env struct {
		Data []struct{ Namespace string `json:"namespace"` } `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Len(t, env.Data, 1)
	assert.Equal(t, "github.com/user/repo", env.Data[0].Namespace)
}

func TestQuiverList_ServiceError(t *testing.T) {
	r := setup(&mocks.QuiverService{ListErr: apperrors.ErrFetchFailed})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/quiver", nil))
	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestQuiverGet_OK(t *testing.T) {
	svc := &mocks.QuiverService{
		GetResult: &appquiver.QuiverDetailDTO{
			Namespace: domain.Namespace("github.com/user/repo"),
			Manifest:  domain.QuiverManifest{Name: "My Quiver"},
		},
	}
	r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, encodedNS, nil))
	assert.Equal(t, http.StatusOK, w.Code)

	var env struct {
		Data struct{ Namespace string `json:"namespace"` } `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, "github.com/user/repo", env.Data.Namespace)
}

func TestQuiverGet_NotFound(t *testing.T) {
	r := setup(&mocks.QuiverService{GetErr: apperrors.ErrNotFound})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, encodedNS, nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}
