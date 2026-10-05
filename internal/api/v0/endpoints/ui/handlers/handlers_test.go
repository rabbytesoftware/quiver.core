package handlers_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/middleware"
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

// serveWithRecovery runs the route behind the real RequestRecovery middleware
// on a real server, so what net/http does with a panic reaches the client.
func serveWithRecovery(t *testing.T, svc *mocks.SurfaceService) *httptest.Server {
	t.Helper()
	r := gin.New()
	r.Use(middleware.RequestRecovery())
	r.Any("/v0/ui/:ns/*path", handlers.New(svc).Serve)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func TestServe_AbortedProxyAbortsTheConnection(t *testing.T) {
	svc := &mocks.SurfaceService{HandlerResult: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	})}
	srv := serveWithRecovery(t, svc)

	resp, err := srv.Client().Get(srv.URL + "/v0/ui/a%2Fb/")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)

	require.ErrorIs(t, err, io.ErrUnexpectedEOF, "a truncated body must not read as complete")
	require.Equal(t, "partial", string(body))
}

func TestServe_OtherPanicsBecome500ThroughRecovery(t *testing.T) {
	svc := &mocks.SurfaceService{HandlerResult: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("real bug")
	})}
	srv := serveWithRecovery(t, svc)

	resp, err := srv.Client().Get(srv.URL + "/v0/ui/a%2Fb/")
	require.NoError(t, err)
	_ = resp.Body.Close()

	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
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
