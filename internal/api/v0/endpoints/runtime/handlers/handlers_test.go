package handlers_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/api/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/runtime/handlers"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

const encodedNS = "/v0/runtime/github.com%2Fuser%2Frepo"

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
