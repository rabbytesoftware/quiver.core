package system_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/system"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func TestRegister_MountsConfigRoutes(t *testing.T) {
	r := gin.New()
	system.Register(r.Group(""), nil)

	mounted := make(map[string]bool)
	for _, route := range r.Routes() {
		mounted[route.Method+" "+route.Path] = true
	}

	assert.True(t, mounted["GET /config"])
	assert.True(t, mounted["PATCH /config"])
}

func TestRegister_UnmountedMethodIsNotFound(t *testing.T) {
	r := gin.New()
	system.Register(r.Group(""), nil)

	req := httptest.NewRequest(http.MethodDelete, "/config", strings.NewReader(""))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
