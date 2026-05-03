package quivers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver/internal/api/mocks"
	"github.com/rabbytesoftware/quiver/internal/app/models"
	"github.com/rabbytesoftware/quiver/internal/domain"
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

func stubWS(c *gin.Context) { c.Status(http.StatusOK) }

func TestRegister_MountsAllRoutes(t *testing.T) {
	svc := &mocks.QuiverService{
		GetResult: &models.QuiverDetailDTO{
			Namespace: domain.Namespace("github.com/user/repo"),
			Name:      "My Quiver",
		},
		ValidateResult: &models.ValidationResult{Valid: true},
	}

	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	Register(r.Group(""), svc, stubWS)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/quiver/github.com%2Fuser%2Frepo"},
		{http.MethodPatch, "/quiver/github.com%2Fuser%2Frepo"},
		{http.MethodDelete, "/quiver/github.com%2Fuser%2Frepo"},
		{http.MethodGet, "/quiver"},
		{http.MethodGet, "/quiver/github.com%2Fuser%2Frepo"},
		{http.MethodPost, "/quiver/github.com%2Fuser%2Frepo/follow"},
		{http.MethodDelete, "/quiver/github.com%2Fuser%2Frepo/follow"},
		{http.MethodGet, "/quiver/github.com%2Fuser%2Frepo/manifest"},
		{http.MethodPost, "/quiver/github.com%2Fuser%2Frepo/manifest"},
		{http.MethodPost, "/quiver/github.com%2Fuser%2Frepo/manifest/validate"},
	}

	for _, tc := range routes {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, nil)
		r.ServeHTTP(w, req)
		assert.NotEqual(t, http.StatusNotFound, w.Code, "%s %s should not 404", tc.method, tc.path)
	}
}
