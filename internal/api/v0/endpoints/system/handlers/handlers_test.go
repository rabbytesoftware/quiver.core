package system_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	systemhandlers "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/system/handlers"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	"github.com/rabbytesoftware/quiver.core/internal/core/config"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

type stubConfigUsecase struct {
	view     usecases.ConfigView
	result   usecases.PatchResult
	getErr   error
	patchErr error
	lastBody string
}

func (s *stubConfigUsecase) Get(
	_ context.Context,
) (usecases.ConfigView, error) {
	if s.getErr != nil {
		return usecases.ConfigView{}, s.getErr
	}
	return s.view, nil
}

func (s *stubConfigUsecase) Patch(
	_ context.Context,
	body json.RawMessage,
) (usecases.PatchResult, error) {
	s.lastBody = string(body)
	if s.patchErr != nil {
		return usecases.PatchResult{}, s.patchErr
	}
	return s.result, nil
}

type stubPathUsecase struct {
	statusResult models.PathStatus
	statusErr    error
	setupResult  models.PathStatus
	setupErr     error
}

func (s *stubPathUsecase) Status(
	_ context.Context,
) (models.PathStatus, error) {
	return s.statusResult, s.statusErr
}

func (s *stubPathUsecase) Setup(
	_ context.Context,
) (models.PathStatus, error) {
	return s.setupResult, s.setupErr
}

func newRouter(svc usecases.ConfigUsecase) *gin.Engine {
	return newRouterWithPath(svc, &stubPathUsecase{})
}

func newRouterWithPath(svc usecases.ConfigUsecase, pathSvc usecases.PathUsecase) *gin.Engine {
	r := gin.New()
	h := systemhandlers.New(svc, pathSvc)
	r.GET("/config", h.Config)
	r.PATCH("/config", h.PatchConfig)
	r.GET("/system/path", h.PathStatus)
	r.POST("/system/path", h.SetupPath)
	return r
}

func do(r *gin.Engine, method, body string) *httptest.ResponseRecorder {
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}

	req := httptest.NewRequest(method, "/config", reader)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func doPath(r *gin.Engine, method string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/system/path", strings.NewReader(""))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeData(t *testing.T, w *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()

	var envelope struct {
		Success bool                       `json:"success"`
		Data    map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	return envelope.Data
}

func TestConfig_ReturnsAllFourSections(t *testing.T) {
	svc := &stubConfigUsecase{view: usecases.ConfigView{
		Running:         config.Defaults(),
		Configured:      config.Defaults(),
		Defaults:        config.Defaults(),
		RestartRequired: []string{"vault.ttl"},
	}}

	w := do(newRouter(svc), http.MethodGet, "")

	require.Equal(t, http.StatusOK, w.Code)
	data := decodeData(t, w)
	assert.Contains(t, data, "running")
	assert.Contains(t, data, "configured")
	assert.Contains(t, data, "defaults")
	assert.JSONEq(t, `["vault.ttl"]`, string(data["restart_required"]))
}

func TestConfig_RunningOmitsHost(t *testing.T) {
	svc := &stubConfigUsecase{view: usecases.ConfigView{
		Running:    config.Defaults(),
		Configured: config.Defaults(),
		Defaults:   config.Defaults(),
	}}

	w := do(newRouter(svc), http.MethodGet, "")

	require.Equal(t, http.StatusOK, w.Code)
	data := decodeData(t, w)
	assert.NotContains(t, string(data["running"]), `"api"`)
	assert.Contains(t, string(data["running"]), `"netbridge"`)
	assert.Contains(t, string(data["configured"]), `"api"`)
}

func TestConfig_EmptyRestartRequiredSerialisesAsArray(t *testing.T) {
	svc := &stubConfigUsecase{view: usecases.ConfigView{
		Running:    config.Defaults(),
		Configured: config.Defaults(),
		Defaults:   config.Defaults(),
	}}

	w := do(newRouter(svc), http.MethodGet, "")

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `[]`, string(decodeData(t, w)["restart_required"]))
}

func TestConfig_UsecaseErrorReturns500(t *testing.T) {
	svc := &stubConfigUsecase{getErr: errors.New("disk on fire")}

	w := do(newRouter(svc), http.MethodGet, "")

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestPatchConfig_AppliesAndReports(t *testing.T) {
	svc := &stubConfigUsecase{result: usecases.PatchResult{
		Applied:  []string{"vault.ttl"},
		Rejected: []config.FieldError{{Key: "logger.level", Message: "must be one of"}},
	}}

	w := do(newRouter(svc), http.MethodPatch,
		`{"vault":{"ttl":"48h"},"logger":{"level":"banana"}}`)

	require.Equal(t, http.StatusOK, w.Code)
	data := decodeData(t, w)
	assert.JSONEq(t, `["vault.ttl"]`, string(data["applied"]))
	assert.Contains(t, string(data["rejected"]), "logger.level")

	assert.Contains(t, svc.lastBody, `"ttl":"48h"`)
}

func TestPatchConfig_ForwardsBodyVerbatim(t *testing.T) {
	svc := &stubConfigUsecase{result: usecases.PatchResult{Applied: []string{"vault.ttl"}}}

	w := do(newRouter(svc), http.MethodPatch, `{"vault":{"ttl":null}}`)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"vault":{"ttl":null}}`, svc.lastBody)
}

func TestPatchConfig_EmptyResultSerialisesAsArrays(t *testing.T) {
	svc := &stubConfigUsecase{}

	w := do(newRouter(svc), http.MethodPatch, `{}`)

	require.Equal(t, http.StatusOK, w.Code)
	data := decodeData(t, w)
	assert.JSONEq(t, `[]`, string(data["applied"]))
	assert.JSONEq(t, `[]`, string(data["rejected"]))
}

func TestPatchConfig_AllRejectedReturns422(t *testing.T) {
	svc := &stubConfigUsecase{patchErr: fmt.Errorf(
		"patch config: %w: logger.level: must be one of", apperrors.ErrInvalidConfig,
	)}

	w := do(newRouter(svc), http.MethodPatch, `{"logger":{"level":"banana"}}`)

	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.Contains(t, w.Body.String(), "logger.level")
}

// Malformed and mistyped bodies are the usecase's judgement now: it reports
// them per setting, so the handler forwards rather than pre-screening.
func TestPatchConfig_MalformedBodyIsForwarded(t *testing.T) {
	svc := &stubConfigUsecase{patchErr: fmt.Errorf(
		"patch config: %w: body must be a json configuration object", apperrors.ErrInvalidConfig,
	)}

	w := do(newRouter(svc), http.MethodPatch, `{"vault":`)

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestPathStatus_ReturnsShelfStatus(t *testing.T) {
	pathSvc := &stubPathUsecase{statusResult: models.PathStatus{
		BinDir: "/home/u/.quiver/bin", OnPath: true, Configured: true, Files: []string{"/home/u/.zshrc"},
	}}

	w := doPath(newRouterWithPath(&stubConfigUsecase{}, pathSvc), http.MethodGet)

	require.Equal(t, http.StatusOK, w.Code)
	data := decodeData(t, w)
	assert.JSONEq(t, `"/home/u/.quiver/bin"`, string(data["bin_dir"]))
	assert.JSONEq(t, `true`, string(data["on_path"]))
	assert.JSONEq(t, `["/home/u/.zshrc"]`, string(data["files"]))
}

func TestPathStatus_UsecaseErrorReturns500(t *testing.T) {
	pathSvc := &stubPathUsecase{statusErr: errors.New("disk on fire")}

	w := doPath(newRouterWithPath(&stubConfigUsecase{}, pathSvc), http.MethodGet)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSetupPath_ReturnsUpdatedStatus(t *testing.T) {
	pathSvc := &stubPathUsecase{setupResult: models.PathStatus{
		BinDir: "/home/u/.quiver/bin", OnPath: true, Configured: true,
	}}

	w := doPath(newRouterWithPath(&stubConfigUsecase{}, pathSvc), http.MethodPost)

	require.Equal(t, http.StatusOK, w.Code)
	data := decodeData(t, w)
	assert.JSONEq(t, `true`, string(data["configured"]))
}

func TestSetupPath_UsecaseErrorReturns500(t *testing.T) {
	pathSvc := &stubPathUsecase{setupErr: errors.New("permission denied")}

	w := doPath(newRouterWithPath(&stubConfigUsecase{}, pathSvc), http.MethodPost)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
