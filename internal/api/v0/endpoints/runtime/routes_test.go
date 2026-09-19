package runtime

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/api/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	domainRuntime "github.com/rabbytesoftware/quiver.core/internal/domain/runtime"
)

func stubWS(c *gin.Context) { c.Status(http.StatusOK) }

func TestRegister_MountsAllRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	svc := &mocks.RuntimeService{
		GetRuntimeResult: &domainRuntime.ArrowRuntime{
			Ref:   "github.com/user/repo",
			State: domain.ArrowStateReady,
		},
	}
	Register(r.Group(""), svc, stubWS)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/runtime/github.com%2Fuser%2Frepo/run"},
		{http.MethodGet, "/runtime"},
		{http.MethodGet, "/runtime/github.com%2Fuser%2Frepo"},
	}

	for _, tc := range routes {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, nil)
		r.ServeHTTP(w, req)
		assert.NotEqual(t, http.StatusNotFound, w.Code, "%s %s should not 404", tc.method, tc.path)
	}
}

func TestRegister_GetWithoutUpgradeServesREST(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	Register(r.Group(""), &mocks.RuntimeService{}, stubWS)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/runtime", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"success":true`)
}

func TestRegister_GetNsWithoutUpgradeServesREST(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	svc := &mocks.RuntimeService{
		GetRuntimeResult: &domainRuntime.ArrowRuntime{
			Ref:   "github.com/user/repo",
			State: domain.ArrowStateReady,
		},
	}
	Register(r.Group(""), svc, stubWS)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/runtime/github.com%2Fuser%2Frepo", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"state":"ready"`)
}

func TestRegister_GetWithUpgradeServesWS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	Register(r.Group(""), &mocks.RuntimeService{}, func(c *gin.Context) {
		c.String(http.StatusTeapot, "ws-handler")
	})

	req := httptest.NewRequest(http.MethodGet, "/runtime", nil)
	req.Header.Set("Upgrade", "websocket")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTeapot, w.Code)
	assert.Equal(t, "ws-handler", w.Body.String())
}
