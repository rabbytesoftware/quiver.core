package health_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	health "github.com/rabbytesoftware/quiver/internal/api/v0/endpoints/health"
	"github.com/stretchr/testify/assert"
)

func TestRegister_HealthRouteResponds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	health.Register(r.Group("/"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	assert.Equal(t, http.StatusOK, w.Code)
}
