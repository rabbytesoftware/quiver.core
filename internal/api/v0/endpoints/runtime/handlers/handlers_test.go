package handlers_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/api/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/runtime/handlers"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

const encodedNS = "/v0/runtime/github.com%2Fuser%2Frepo"

var errTestBoom = errors.New("boom")

func setup(rt *mocks.RuntimeService) (*handlers.Handlers, *gin.Engine) {
	if rt == nil {
		rt = &mocks.RuntimeService{}
	}
	h := handlers.New(rt)
	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	r.POST("/v0/runtime/:ns/:method", h.Execute)
	return h, r
}

func TestExecute_Accepted(t *testing.T) {
	_, r := setup(nil)
	body := bytes.NewBufferString(`{"variables":{"KEY":"val"}}`)
	req := httptest.NewRequest(http.MethodPost, "/v0/runtime/github.com%2Fuser%2Frepo/run", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusAccepted, w.Code)
}

func TestExecute_StateViolation(t *testing.T) {
	rt := &mocks.RuntimeService{ExecuteErr: apperrors.ErrStateViolation}
	_, r := setup(rt)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v0/runtime/github.com%2Fuser%2Frepo/run", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestInstall_Accepted(t *testing.T) {
	_, r := setup(nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS+"/install", nil))
	assert.Equal(t, http.StatusAccepted, w.Code)
}

func TestInstall_StateViolation(t *testing.T) {
	rt := &mocks.RuntimeService{InstallErr: apperrors.ErrStateViolation}
	_, r := setup(rt)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS+"/install", nil))
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestUninstall_Accepted(t *testing.T) {
	_, r := setup(nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS+"/uninstall", nil))
	assert.Equal(t, http.StatusAccepted, w.Code)
}

func TestUninstall_StateViolation(t *testing.T) {
	rt := &mocks.RuntimeService{UninstallErr: apperrors.ErrStateViolation}
	_, r := setup(rt)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS+"/uninstall", nil))
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestUninstall_DependentsExist(t *testing.T) {
	rt := &mocks.RuntimeService{UninstallErr: apperrors.ErrDependentsExist}
	_, r := setup(rt)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS+"/uninstall", nil))
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestStop_Accepted(t *testing.T) {
	_, r := setup(nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS+"/stop", nil))
	assert.Equal(t, http.StatusAccepted, w.Code)
}

func TestStop_StateViolation(t *testing.T) {
	rt := &mocks.RuntimeService{StopErr: apperrors.ErrStateViolation}
	_, r := setup(rt)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS+"/stop", nil))
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestExecute_MethodNotFound_Returns404(t *testing.T) {
	rt := &mocks.RuntimeService{ExecuteErr: apperrors.ErrMethodNotFound}
	_, r := setup(rt)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v0/runtime/github.com%2Fuser%2Frepo/nonexistent", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestExecute_LifecycleExecute_Accepted(t *testing.T) {
	_, r := setup(nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS+"/execute", nil))
	assert.Equal(t, http.StatusAccepted, w.Code)
}

func TestExecute_LifecycleExecute_StateViolation(t *testing.T) {
	rt := &mocks.RuntimeService{ExecuteErr: apperrors.ErrStateViolation}
	_, r := setup(rt)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS+"/execute", nil))
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestInstall_MethodNotFound_Returns404(t *testing.T) {
	rt := &mocks.RuntimeService{InstallErr: apperrors.ErrMethodNotFound}
	_, r := setup(rt)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS+"/install", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ─── GET /runtime/:ns and GET /runtime ──────────────────────────────────────

func setupGet(rt *mocks.RuntimeService) *gin.Engine {
	if rt == nil {
		rt = &mocks.RuntimeService{}
	}
	h := handlers.New(rt)
	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	r.GET("/v0/runtime", h.List)
	r.GET("/v0/runtime/:ns", h.Get)
	return r
}

func TestGet_ReturnsRuntime(t *testing.T) {
	rt := &mocks.RuntimeService{
		GetRuntimeResult: &domainRuntime.ArrowRuntime{
			Ref:   "github.com/user/repo",
			State: domain.ArrowStateRunning,
		},
	}
	r := setupGet(rt)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, encodedNS, nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"success":true`)
	assert.Contains(t, w.Body.String(), `"state":"running"`)
	assert.Contains(t, w.Body.String(), `"namespace":"github.com/user/repo"`)
}

func TestGet_NotFound(t *testing.T) {
	rt := &mocks.RuntimeService{GetRuntimeErr: apperrors.ErrNotFound}
	r := setupGet(rt)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, encodedNS, nil))

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), `"success":false`)
}

func TestGet_InternalError(t *testing.T) {
	rt := &mocks.RuntimeService{GetRuntimeErr: errTestBoom}
	r := setupGet(rt)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, encodedNS, nil))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestList_ReturnsRuntimes(t *testing.T) {
	rt := &mocks.RuntimeService{
		ListRuntimesResult: []domainRuntime.ArrowRuntime{
			{Ref: "github.com/user/a@v1", State: domain.ArrowStateRunning},
			{Ref: "github.com/user/b@v1", State: domain.ArrowStateAbsent},
		},
	}
	r := setupGet(rt)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v0/runtime", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"github.com/user/a@v1"`)
	assert.Contains(t, w.Body.String(), `"github.com/user/b@v1"`)
}

func TestList_EmptyReturnsArray(t *testing.T) {
	r := setupGet(nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v0/runtime", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"data":[]`)
}

func TestList_InternalError(t *testing.T) {
	rt := &mocks.RuntimeService{ListRuntimesErr: errTestBoom}
	r := setupGet(rt)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v0/runtime", nil))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGet_NilRuntimeReturnsNotFound(t *testing.T) {
	r := setupGet(&mocks.RuntimeService{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, encodedNS, nil))

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), `"success":false`)
}
