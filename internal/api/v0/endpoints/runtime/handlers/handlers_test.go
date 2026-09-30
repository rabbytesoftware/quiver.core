package handlers_test

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
	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/runtime/handlers"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	ucmocks "github.com/rabbytesoftware/quiver.core/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
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
	rt := &mocks.RuntimeService{InstallStarted: true}
	_, r := setup(rt)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS+"/install", nil))
	assert.Equal(t, http.StatusAccepted, w.Code)
}

func TestInstall_NoOp_Returns200(t *testing.T) {
	rt := &mocks.RuntimeService{InstallStarted: false}
	_, r := setup(rt)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS+"/install", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestInstall_StateViolation(t *testing.T) {
	rt := &mocks.RuntimeService{InstallErr: apperrors.ErrStateViolation}
	_, r := setup(rt)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS+"/install", nil))
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestUpdate_StatusFollowsWhetherAnUpdateStarted(t *testing.T) {
	testCases := []struct {
		name    string
		method  string
		started bool
		err     error
		want    int
	}{
		{name: "an update that started", method: "update", started: true, want: http.StatusAccepted},
		{name: "nothing newer is an idempotent no-op", method: "update", want: http.StatusOK},
		{name: "the legacy _update spelling", method: "_update", want: http.StatusOK},
		{name: "a rejected bracket", method: "update", err: apperrors.ErrStateViolation, want: http.StatusUnprocessableEntity},
		{name: "a row that is not catalogued", method: "update", err: apperrors.ErrNotFound, want: http.StatusNotFound},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, r := setup(&mocks.RuntimeService{UpdateStarted: tc.started, UpdateErr: tc.err})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, encodedNS+"/"+tc.method, nil)

			r.ServeHTTP(w, req)

			assert.Equal(t, tc.want, w.Code)
		})
	}
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

// ─── reserved variables ──────────────────────────────────────────────────────

// Wired to the real usecase rather than a stub: the point is that a request
// setting a built-in is refused before anything is executed, and that the
// client is told which variable was refused rather than silently ignored.
func setupWithRealUsecase() *gin.Engine {
	uc := usecases.NewRuntimeUsecase(
		&ucmocks.MockArrow{},
		&ucmocks.MockRuntime{},
		&ucmocks.MockGraph{},
	)
	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	r.POST("/v0/runtime/:ns/:method", handlers.New(uc).Execute)
	return r
}

func TestExecute_ReservedVariable_Returns400NamingIt(t *testing.T) {
	methods := []string{"install", "uninstall", "execute", "update", "custom"}

	for _, method := range methods {
		for _, name := range domain.ReservedVariableNames() {
			t.Run(method+"/"+name, func(t *testing.T) {
				r := setupWithRealUsecase()
				body := bytes.NewBufferString(`{"variables":{"` + name + `":"hijacked"}}`)
				req := httptest.NewRequest(http.MethodPost, encodedNS+"/"+method, body)
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()

				r.ServeHTTP(w, req)

				assert.Equal(t, http.StatusBadRequest, w.Code)

				var env struct {
					Success bool   `json:"success"`
					Error   string `json:"error"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
				assert.False(t, env.Success)
				assert.Contains(t, env.Error, name)
			})
		}
	}
}

func TestExecute_NonReservedVariable_IsAccepted(t *testing.T) {
	r := setupWithRealUsecase()
	body := bytes.NewBufferString(`{"variables":{"PORT":"8080"}}`)
	req := httptest.NewRequest(http.MethodPost, encodedNS+"/execute", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
}

// ─── settling ────────────────────────────────────────────────────────────────

func TestRuntimeReads_ReportSettlingRows(t *testing.T) {
	const settlingNS = domain.Namespace("github.com/user/repo")
	svc := &mocks.RuntimeService{
		GetRuntimeResult: &domainRuntime.ArrowRuntime{Ref: settlingNS, State: domain.ArrowStateOutdated},
		ListRuntimesResult: []domainRuntime.ArrowRuntime{
			{Ref: settlingNS, State: domain.ArrowStateOutdated},
			{Ref: "github.com/user/other", State: domain.ArrowStateReady},
		},
		SettlingNamespaces: map[domain.Namespace]bool{settlingNS: true},
	}
	h := handlers.New(svc)
	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	r.GET("/v0/runtime", h.List)
	r.GET("/v0/runtime/:ns", h.Get)

	testCases := []struct {
		name string
		path string
		want map[string]bool
	}{
		{name: "one runtime", path: encodedNS, want: map[string]bool{"github.com/user/repo": true}},
		{name: "every runtime", path: "/v0/runtime", want: map[string]bool{
			"github.com/user/repo":  true,
			"github.com/user/other": false,
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
			require.Equal(t, http.StatusOK, w.Code)

			got := map[string]bool{}
			for _, rt := range decodeRuntimes(t, w.Body.Bytes()) {
				got[rt.Namespace] = rt.Settling
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func decodeRuntimes(t *testing.T, body []byte) []apidto.ArrowRuntimeDTO {
	t.Helper()
	var list struct {
		Data []apidto.ArrowRuntimeDTO `json:"data"`
	}
	if err := json.Unmarshal(body, &list); err == nil {
		return list.Data
	}
	var one struct {
		Data apidto.ArrowRuntimeDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &one))
	return []apidto.ArrowRuntimeDTO{one.Data}
}
