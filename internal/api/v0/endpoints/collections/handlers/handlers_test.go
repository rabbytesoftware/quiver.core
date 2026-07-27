package quivers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/mocks"
	quivers "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/collections/handlers"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

const encodedNS = "/v0/collection/github.com%2Fuser%2Frepo"

func setup(svc *mocks.CollectionService) *gin.Engine {
	h := quivers.New(svc)
	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	r.POST("/v0/collection/:ns/follow", h.Follow)
	r.DELETE("/v0/collection/:ns/follow", h.Unfollow)
	r.GET("/v0/collection", h.List)
	r.GET("/v0/collection/:ns", h.Get)
	r.GET("/v0/collection/:ns/manifest", h.GetManifest)
	r.POST("/v0/collection/:ns/manifest", h.SeedManifest)
	r.POST("/v0/collection/:ns/manifest/validate", h.ValidateManifest)
	return r
}

func TestQuiverFollow_Created(t *testing.T) {
	r := setup(&mocks.CollectionService{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v0/collection/github.com%2Fuser%2Frepo/follow", nil))
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestQuiverFollow_Conflict(t *testing.T) {
	r := setup(&mocks.CollectionService{FollowErr: apperrors.ErrAlreadyExists})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v0/collection/github.com%2Fuser%2Frepo/follow", nil))
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestQuiverFollow_NotFound(t *testing.T) {
	r := setup(&mocks.CollectionService{FollowErr: apperrors.ErrNotFound})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v0/collection/github.com%2Fuser%2Frepo/follow", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestQuiverUnfollow_OK(t *testing.T) {
	r := setup(&mocks.CollectionService{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/v0/collection/github.com%2Fuser%2Frepo/follow", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestQuiverUnfollow_NotFound(t *testing.T) {
	r := setup(&mocks.CollectionService{UnfollowErr: apperrors.ErrNotFound})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/v0/collection/github.com%2Fuser%2Frepo/follow", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestQuiverList_OK(t *testing.T) {
	svc := &mocks.CollectionService{
		ListResult: []models.CollectionListDTO{
			{Namespace: domain.Namespace("github.com/user/repo"), Name: "My Quiver"},
		},
	}
	r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v0/collection", nil))
	assert.Equal(t, http.StatusOK, w.Code)

	var env struct {
		Data []struct {
			Namespace string `json:"namespace"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Len(t, env.Data, 1)
	assert.Equal(t, "github.com/user/repo", env.Data[0].Namespace)
}

func TestQuiverList_FollowedFilter(t *testing.T) {
	svc := &mocks.CollectionService{
		ListResult: []models.CollectionListDTO{
			{Namespace: domain.Namespace("github.com/user/repo"), Followed: true},
		},
	}
	r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v0/collection?followed=true", nil))
	assert.Equal(t, http.StatusOK, w.Code)

	var env struct {
		Data []struct {
			Followed bool `json:"followed"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.Len(t, env.Data, 1)
	assert.True(t, env.Data[0].Followed)
}

func TestQuiverList_ServiceError(t *testing.T) {
	r := setup(&mocks.CollectionService{ListErr: apperrors.ErrFetchFailed})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v0/collection", nil))
	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestQuiverGet_OK(t *testing.T) {
	svc := &mocks.CollectionService{
		GetResult: &models.CollectionDetailDTO{
			Namespace: domain.Namespace("github.com/user/repo"),
			Name:      "My Quiver",
		},
	}
	r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, encodedNS, nil))
	assert.Equal(t, http.StatusOK, w.Code)

	var env struct {
		Data struct {
			Namespace string `json:"namespace"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, "github.com/user/repo", env.Data.Namespace)
}

func TestQuiverGet_NotFound(t *testing.T) {
	r := setup(&mocks.CollectionService{GetErr: apperrors.ErrNotFound})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, encodedNS, nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestQuiverSeedManifest_Created(t *testing.T) {
	r := setup(&mocks.CollectionService{})
	body := bytes.NewBufferString(`{"name":"test"}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS+"/manifest", body))
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestQuiverSeedManifest_ServiceError(t *testing.T) {
	r := setup(&mocks.CollectionService{SeedErr: apperrors.ErrNotFound})
	body := bytes.NewBufferString(`{"name":"test"}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS+"/manifest", body))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// A manifest the parser rejects is the caller's mistake, so it must not read
// as an internal error.
func TestQuiverSeedManifest_InvalidManifest_Returns422(t *testing.T) {
	r := setup(&mocks.CollectionService{SeedErr: apperrors.ErrInvalidManifest})
	body := bytes.NewBufferString(`{"name":"test"}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS+"/manifest", body))
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestQuiverGetManifest_OK(t *testing.T) {
	raw := []byte(`{"name":"test"}`)
	r := setup(&mocks.CollectionService{GetManifestResult: raw})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, encodedNS+"/manifest", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, raw, w.Body.Bytes())
}

func TestQuiverGetManifest_NotFound(t *testing.T) {
	r := setup(&mocks.CollectionService{GetManifestErr: apperrors.ErrNotFound})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, encodedNS+"/manifest", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestQuiverValidateManifest_Valid(t *testing.T) {
	r := setup(&mocks.CollectionService{
		ValidateResult: &models.ValidationResult{Valid: true},
	})
	body := bytes.NewBufferString(`{"name":"test"}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS+"/manifest/validate", body))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestQuiverValidateManifest_Invalid(t *testing.T) {
	r := setup(&mocks.CollectionService{
		ValidateResult: &models.ValidationResult{
			Valid:  false,
			Errors: []models.ValidationError{{Field: "name", Rule: "required", Message: "name is required"}},
		},
	})
	body := bytes.NewBufferString(`{}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS+"/manifest/validate", body))
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestQuiverValidateManifest_ServiceError(t *testing.T) {
	r := setup(&mocks.CollectionService{ValidateErr: apperrors.ErrFetchFailed})
	body := bytes.NewBufferString(`{}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS+"/manifest/validate", body))
	assert.Equal(t, http.StatusBadGateway, w.Code)
}
