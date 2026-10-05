package handlers_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/ui/handlers"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

func setup(svc *mocks.SurfaceService) *gin.Engine {
	h := handlers.New(svc)
	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	r.Any("/v0/ui/:ns/*path", h.Serve)
	return r
}

func echoPath() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("path=" + r.URL.Path + " query=" + r.URL.RawQuery))
	})
}

func TestServe_StripsPrefixAndProxies(t *testing.T) {
	svc := &mocks.SurfaceService{HandlerResult: echoPath()}
	rec := httptest.NewRecorder()
	setup(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v0/ui/github.com%2Fuser%2Fchat/assets/app.js", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "path=/assets/app.js query=", rec.Body.String())
}

func TestServe_ForwardsQueryUntouched(t *testing.T) {
	svc := &mocks.SurfaceService{HandlerResult: echoPath()}
	rec := httptest.NewRecorder()
	setup(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v0/ui/a%2Fb/?x=1&y=a%2Fb&x=2", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "path=/ query=x=1&y=a%2Fb&x=2", rec.Body.String())
}

func TestServe_BareNamespaceRedirectsToTrailingSlash(t *testing.T) {
	svc := &mocks.SurfaceService{HandlerResult: echoPath()}
	rec := httptest.NewRecorder()
	setup(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v0/ui/a%2Fb?x=1", nil))

	require.Equal(t, http.StatusMovedPermanently, rec.Code)
	require.Contains(t, rec.Header().Get("Location"), "/v0/ui/")
}

func TestServe_NoSurfaceIs503(t *testing.T) {
	svc := &mocks.SurfaceService{HandlerErr: usecases.ErrNoSurface}
	rec := httptest.NewRecorder()
	setup(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v0/ui/a%2Fb/", nil))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestServe_UnknownArrowIs404(t *testing.T) {
	svc := &mocks.SurfaceService{HandlerErr: apperrors.ErrNotFound}
	rec := httptest.NewRecorder()
	setup(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v0/ui/a%2Fb/", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestServe_OtherErrorIs500(t *testing.T) {
	svc := &mocks.SurfaceService{HandlerErr: errors.New("boom")}
	rec := httptest.NewRecorder()
	setup(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v0/ui/a%2Fb/", nil))
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestServe_AbortedProxyDoesNotPanic(t *testing.T) {
	svc := &mocks.SurfaceService{HandlerResult: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	})}
	rec := httptest.NewRecorder()
	require.NotPanics(t, func() {
		setup(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v0/ui/a%2Fb/", nil))
	})
}

func TestServe_OtherPanicsStillPropagate(t *testing.T) {
	svc := &mocks.SurfaceService{HandlerResult: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("real bug")
	})}
	rec := httptest.NewRecorder()
	require.Panics(t, func() {
		setup(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v0/ui/a%2Fb/", nil))
	})
}
