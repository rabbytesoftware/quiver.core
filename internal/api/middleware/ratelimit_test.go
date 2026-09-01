package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRateLimitTestRouter(limiter *RateLimiter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/limited", limiter.Middleware(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

func TestRateLimiter_AllowsUpToLimit(t *testing.T) {
	limiter := NewRateLimiter(3, time.Minute)
	r := newRateLimitTestRouter(limiter)

	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/limited", nil))
		assert.Equal(t, http.StatusOK, w.Code, "request %d should be allowed", i+1)
	}
}

func TestRateLimiter_RejectsOverLimit(t *testing.T) {
	limiter := NewRateLimiter(2, time.Minute)
	r := newRateLimitTestRouter(limiter)

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/limited", nil))
		require.Equal(t, http.StatusOK, w.Code)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/limited", nil))
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestRateLimiter_ResetsAfterWindow(t *testing.T) {
	now := time.Now()
	limiter := NewRateLimiter(1, time.Minute)
	limiter.now = func() time.Time { return now }

	require.True(t, limiter.allow())
	require.False(t, limiter.allow(), "second call within the window must be rejected")

	now = now.Add(2 * time.Minute)
	assert.True(t, limiter.allow(), "a call after the window has elapsed must be allowed again")
}

func TestRateLimiter_GlobalAcrossCallers(t *testing.T) {
	// The limiter counts requests globally, not per caller — two different
	// remote addresses share the same budget, which is the whole point: a
	// brute-force search across the pairing-code space cannot be spread
	// across source addresses to dodge the limit.
	limiter := NewRateLimiter(1, time.Minute)
	r := newRateLimitTestRouter(limiter)

	req1 := httptest.NewRequest(http.MethodPost, "/limited", nil)
	req1.RemoteAddr = "203.0.113.1:1"
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	require.Equal(t, http.StatusOK, w1.Code)

	req2 := httptest.NewRequest(http.MethodPost, "/limited", nil)
	req2.RemoteAddr = "203.0.113.2:1"
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusTooManyRequests, w2.Code)
}
