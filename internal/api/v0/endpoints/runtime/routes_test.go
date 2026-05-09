package runtime

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/api/mocks"
)

func stubWS(c *gin.Context) { c.Status(http.StatusOK) }

func TestRegister_MountsAllRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	Register(r.Group(""), &mocks.RuntimeService{}, stubWS)

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
