package search

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func TestRegister_MountsSearchRoute(t *testing.T) {
	svc := &mocks.SearchService{}
	r := gin.New()
	Register(r.Group(""), svc, &mocks.DiscoveryService{}, func(*gin.Context) {})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/search?q=redis", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, svc.SearchCalls)
}

func TestRegister_MountsDiscoverRoute(t *testing.T) {
	disc := &mocks.DiscoveryService{StartResult: usecases.Job{ID: "job-1"}}
	r := gin.New()
	Register(r.Group(""), &mocks.SearchService{}, disc, func(*gin.Context) {})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/search/discover", strings.NewReader(`{"q":"chrom"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
	assert.Equal(t, 1, disc.StartCalls())
}

func TestRoutes_PlainGETGoesToREST(t *testing.T) {
	disc := &mocks.DiscoveryService{GetResult: &usecases.Job{ID: "job-1", Status: usecases.JobRunning}}
	wsCalls := 0
	r := gin.New()
	Register(r.Group(""), &mocks.SearchService{}, disc, func(*gin.Context) { wsCalls++ })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/search/discover/job-1", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "job-1", disc.GetID())
	assert.Zero(t, wsCalls)
	assert.Empty(t, disc.Cancelled(), "a summary read must not cancel the pass")
}

func TestRoutes_UpgradeHeaderGoesToWS(t *testing.T) {
	disc := &mocks.DiscoveryService{}
	wsCalls := 0
	r := gin.New()
	Register(r.Group(""), &mocks.SearchService{}, disc, func(*gin.Context) { wsCalls++ })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/search/discover/job-1", nil)
	req.Header.Set("Upgrade", "websocket")
	r.ServeHTTP(w, req)

	assert.Equal(t, 1, wsCalls)
	assert.Empty(t, disc.GetID(), "the summary handler must not run for an upgrade")
}

// TestRoutes_SubscriberLeavingCancelsThePass: the broadcaster's handler blocks
// until the socket closes, so returning from it means the subscriber is gone.
func TestRoutes_SubscriberLeavingCancelsThePass(t *testing.T) {
	disc := &mocks.DiscoveryService{}
	r := gin.New()
	Register(r.Group(""), &mocks.SearchService{}, disc, func(*gin.Context) {})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/search/discover/job-1", nil)
	req.Header.Set("Upgrade", "websocket")
	r.ServeHTTP(w, req)

	assert.Equal(t, []string{"job-1"}, disc.Cancelled())
}

// TestRoutes_WithoutDiscovery_LeavingCancelsNothing guards the container built
// with no vault or manifold: the routes still answer, and the stream teardown
// must not dereference a usecase that does not exist.
func TestRoutes_WithoutDiscovery_LeavingCancelsNothing(t *testing.T) {
	r := gin.New()
	Register(r.Group(""), &mocks.SearchService{}, nil, func(*gin.Context) {})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/search/discover/job-1", nil)
	req.Header.Set("Upgrade", "websocket")

	require.NotPanics(t, func() { r.ServeHTTP(w, req) })
}
