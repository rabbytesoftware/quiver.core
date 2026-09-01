package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/api/middleware"
	"github.com/rabbytesoftware/quiver.core/internal/api/mocks"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func TestRegister_MountsAllRoutes(t *testing.T) {
	svc := &mocks.AuthService{}
	gate := middleware.NewAuthGate(svc)
	limiter := middleware.NewRateLimiter(1000, time.Minute)

	r := gin.New()
	Register(r.Group(""), svc, gate, limiter)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/auth/pairing"},
		{http.MethodPost, "/auth/pairing/redeem"},
		{http.MethodGet, "/auth/devices"},
		{http.MethodDelete, "/auth/devices/dev-1"},
	}

	for _, tc := range routes {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, nil)
		r.ServeHTTP(w, req)
		assert.NotEqual(t, http.StatusNotFound, w.Code, "%s %s should not 404", tc.method, tc.path)
	}
}

func TestRegister_AdminRoutes_LoopbackGated(t *testing.T) {
	svc := &mocks.AuthService{}
	gate := middleware.NewAuthGate(svc)
	gate.SetRequired(true)
	limiter := middleware.NewRateLimiter(1000, time.Minute)

	r := gin.New()
	Register(r.Group(""), svc, gate, limiter)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/pairing", nil)
	req.RemoteAddr = "203.0.113.5:12345"
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRegister_Redeem_NotLoopbackGated(t *testing.T) {
	svc := &mocks.AuthService{}
	gate := middleware.NewAuthGate(svc)
	gate.SetRequired(true)
	limiter := middleware.NewRateLimiter(1000, time.Minute)

	r := gin.New()
	Register(r.Group(""), svc, gate, limiter)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/pairing/redeem", nil)
	req.RemoteAddr = "203.0.113.5:12345"
	r.ServeHTTP(w, req)

	assert.NotEqual(t, http.StatusForbidden, w.Code, "redeem must be reachable from a non-loopback address")
}
