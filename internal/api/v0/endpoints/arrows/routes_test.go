package arrows

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/api/mocks"
	"github.com/rabbytesoftware/quiver/internal/app/arrow"
	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func TestDispatch_PlainHTTP_CallsRestHandler(t *testing.T) {
	restFn := func(c *gin.Context) {
		c.Header("X-Handler", "rest")
		c.Status(http.StatusOK)
	}
	wsFn := func(c *gin.Context) {
		c.Header("X-Handler", "ws")
		c.Status(http.StatusOK)
	}

	r := gin.New()
	r.GET("/test", dispatch(restFn, wsFn))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, "rest", w.Header().Get("X-Handler"))
}

func TestDispatch_WebSocketUpgrade_CallsWSHandler(t *testing.T) {
	restFn := func(c *gin.Context) {
		c.Header("X-Handler", "rest")
		c.Status(http.StatusOK)
	}
	wsFn := func(c *gin.Context) {
		c.Header("X-Handler", "ws")
		c.Status(http.StatusOK)
	}

	r := gin.New()
	r.GET("/test", dispatch(restFn, wsFn))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Upgrade", "websocket")
	r.ServeHTTP(w, req)

	assert.Equal(t, "ws", w.Header().Get("X-Handler"))
}

type stubWSHandler struct{}

func (s *stubWSHandler) HandleArrow(c *gin.Context)        { c.Status(http.StatusOK) }
func (s *stubWSHandler) HandleArrowRuntime(c *gin.Context) { c.Status(http.StatusOK) }

func TestRegister_MountsAllRoutes(t *testing.T) {
	svc := &mocks.ArrowService{
		GetDetailResult: &arrow.ArrowDetailDTO{
			Namespace: domain.Namespace("github.com/user/repo"),
			Manifest:  domain.ArrowManifest{ArrowMeta: domain.ArrowMeta{Name: "Test", Version: "1.0.0"}},
			State:     domain.ArrowStateReady,
		},
	}

	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	Register(r.Group(""), svc, &stubWSHandler{})

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/arrow/github.com%2Fuser%2Frepo"},
		{http.MethodPatch, "/arrow/github.com%2Fuser%2Frepo"},
		{http.MethodDelete, "/arrow/github.com%2Fuser%2Frepo"},
		{http.MethodGet, "/arrow"},
		{http.MethodGet, "/arrow/github.com%2Fuser%2Frepo"},
		{http.MethodPost, "/arrow/github.com%2Fuser%2Frepo/run"},
		{http.MethodGet, "/arrow.runtime"},
		{http.MethodGet, "/arrow.runtime/github.com%2Fuser%2Frepo"},
	}

	for _, tc := range routes {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, nil)
		r.ServeHTTP(w, req)
		assert.NotEqual(t, http.StatusNotFound, w.Code, "%s %s should not 404", tc.method, tc.path)
	}
}
