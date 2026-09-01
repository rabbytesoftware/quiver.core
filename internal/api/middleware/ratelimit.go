package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
)

// RateLimiter is a global fixed-window request counter guarding the pairing
// redeem endpoint.
//
// It is not domain state: it resets on daemon restart and is never
// persisted, so it lives here rather than as an Asynx aggregate. It also
// cannot be replaced by a per-pairing-code attempt counter — a wrong guess
// against the endpoint almost never names a real pending code, so it never
// touches that code's own aggregate to be counted against. Only a limiter on
// the endpoint itself bounds the search across the whole code space.
type RateLimiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	count    int
	resetsAt time.Time
	now      func() time.Time
}

// NewRateLimiter returns a limiter allowing at most limit requests per
// window, counted globally across all callers.
func NewRateLimiter(
	limit int,
	window time.Duration,
) *RateLimiter {
	return &RateLimiter{limit: limit, window: window, now: time.Now}
}

// Middleware rejects a request with 429 once the window's budget is spent.
func (r *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !r.allow() {
			libs.WriteErr(c, http.StatusTooManyRequests, "too many pairing attempts, try again later", "")
			c.Abort()
			return
		}

		c.Next()
	}
}

func (r *RateLimiter) allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	if !now.Before(r.resetsAt) {
		r.count = 0
		r.resetsAt = now.Add(r.window)
	}

	if r.count >= r.limit {
		return false
	}

	r.count++
	return true
}
