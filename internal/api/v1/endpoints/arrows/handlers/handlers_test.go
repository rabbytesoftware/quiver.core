package arrows_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	arrows "github.com/rabbytesoftware/quiver/internal/api/v1/endpoints/arrows/handlers"
	"github.com/rabbytesoftware/quiver/internal/api/mocks"
	"github.com/rabbytesoftware/quiver/internal/app/arrow"
	apperrors "github.com/rabbytesoftware/quiver/internal/app/errors"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NOTE: TestMain is already defined in dtos_test.go (same package) — do NOT add it here.

const encodedNS = "/v1/arrow/github.com%2Fuser%2Frepo"

func setup(svc *mocks.ArrowService) (*arrows.Handlers, *gin.Engine) {
	h := arrows.New(svc)
	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	r.POST("/v1/arrow/:ns", h.Add)
	r.PATCH("/v1/arrow/:ns", h.Update)
	r.DELETE("/v1/arrow/:ns", h.Remove)
	r.GET("/v1/arrow", h.List)
	r.GET("/v1/arrow/:ns", h.GetDetail)
	r.POST("/v1/arrow/:ns/:method", h.Execute)
	return h, r
}

func TestAdd_Created(t *testing.T) {
	svc := &mocks.ArrowService{}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS, nil))
	assert.Equal(t, http.StatusCreated, w.Code)
	assertSuccess(t, w.Body.Bytes())
}

func TestAdd_ServiceError(t *testing.T) {
	svc := &mocks.ArrowService{AddErr: apperrors.ErrAlreadyExists}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS, nil))
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestUpdate_OK(t *testing.T) {
	svc := &mocks.ArrowService{}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, encodedNS, nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUpdate_ServiceError(t *testing.T) {
	svc := &mocks.ArrowService{UpdateErr: apperrors.ErrNotFound}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, encodedNS, nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRemove_OK(t *testing.T) {
	svc := &mocks.ArrowService{}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, encodedNS, nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRemove_DependentsExist(t *testing.T) {
	svc := &mocks.ArrowService{RemoveErr: apperrors.ErrDependentsExist}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, encodedNS, nil))
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestList_OK(t *testing.T) {
	svc := &mocks.ArrowService{
		ListResult: []arrow.ArrowListDTO{
			{
				Namespace: domain.Namespace("github.com/user/repo"),
				Name:      "Test",
				State:     domain.ArrowStateReady,
			},
		},
	}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/arrow", nil))
	assert.Equal(t, http.StatusOK, w.Code)

	var env struct {
		Success bool `json:"success"`
		Data    []struct {
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.True(t, env.Success)
	require.Len(t, env.Data, 1)
	assert.Equal(t, "github.com/user/repo", env.Data[0].Namespace)
}

func TestList_ServiceError(t *testing.T) {
	svc := &mocks.ArrowService{ListErr: apperrors.ErrFetchFailed}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/arrow", nil))
	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestGetDetail_OK(t *testing.T) {
	svc := &mocks.ArrowService{
		GetDetailResult: &arrow.ArrowDetailDTO{
			Namespace: domain.Namespace("github.com/user/repo"),
			Manifest:  domain.ArrowManifest{Name: "Test", Version: "1.0.0"},
			State:     domain.ArrowStateReady,
		},
	}
	_, r := setup(svc)
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

func TestGetDetail_NotFound(t *testing.T) {
	svc := &mocks.ArrowService{GetDetailErr: apperrors.ErrNotFound}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, encodedNS, nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestExecute_Accepted(t *testing.T) {
	svc := &mocks.ArrowService{}
	_, r := setup(svc)
	body := bytes.NewBufferString(`{"variables":{"KEY":"val"}}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/arrow/github.com%2Fuser%2Frepo/run", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusAccepted, w.Code)
}

func TestExecute_StateViolation(t *testing.T) {
	svc := &mocks.ArrowService{BeginExecutionErr: apperrors.ErrStateViolation}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/arrow/github.com%2Fuser%2Frepo/run", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestNamespace_PercentEncoded(t *testing.T) {
	// Verify Gin decodes %2F in path params: /v1/arrow/github.com%2Fuser%2Frepo
	// should yield ns == "github.com/user/repo" to the service.
	var capturedNS domain.Namespace
	svc := &mocks.ArrowService{}
	h := arrows.New(svc)
	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	r.GET("/v1/arrow/:ns", func(c *gin.Context) {
		capturedNS = domain.Namespace(c.Param("ns"))
		h.GetDetail(c)
	})
	svc.GetDetailResult = &arrow.ArrowDetailDTO{
		Namespace: domain.Namespace("github.com/user/repo"),
		Manifest:  domain.ArrowManifest{Name: "Test"},
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/arrow/github.com%2Fuser%2Frepo", nil))
	assert.Equal(t, domain.Namespace("github.com/user/repo"), capturedNS)
}

func assertSuccess(t *testing.T, body []byte) {
	t.Helper()
	var env struct{ Success bool `json:"success"` }
	require.NoError(t, json.Unmarshal(body, &env))
	assert.True(t, env.Success)
}
