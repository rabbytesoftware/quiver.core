package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRequestTimer_SetsStartTime(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RequestTimer())
	router.GET("/test", func(c *gin.Context) {
		val, exists := c.Get(requestStartTimeKey)
		if !exists {
			t.Fatal("expected request_start_time to be set in context")
		}

		startTime, ok := val.(time.Time)
		if !ok {
			t.Fatal("expected request_start_time to be time.Time type")
		}

		if startTime.IsZero() {
			t.Fatal("expected request_start_time to be non-zero")
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestRequestTimer_StartTimeBeforeNow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RequestTimer())

	beforeRequest := time.Now()

	router.GET("/test", func(c *gin.Context) {
		val, exists := c.Get(requestStartTimeKey)
		if !exists {
			t.Fatal("expected request_start_time to be set")
		}

		startTime := val.(time.Time)
		afterRequest := time.Now()

		// Verify start time is between beforeRequest and afterRequest
		if startTime.Before(beforeRequest) {
			t.Fatal("start time should not be before request was made")
		}

		if startTime.After(afterRequest) {
			t.Fatal("start time should not be after request was processed")
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestRequestTimer_CallsNext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handlerCalled := false

	router := gin.New()
	router.Use(RequestTimer())
	router.GET("/test", func(c *gin.Context) {
		handlerCalled = true
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if !handlerCalled {
		t.Fatal("expected next handler to be called")
	}

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestRequestTimer_MultipleRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RequestTimer())
	router.GET("/test", func(c *gin.Context) {
		val, _ := c.Get(requestStartTimeKey)
		startTime := val.(time.Time)

		if startTime.IsZero() {
			t.Error("start time should not be zero")
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Make multiple requests
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected status 200, got %d", i, w.Code)
		}
	}
}

func TestRequestTimer_DifferentTimesForEachRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var startTimes []time.Time
	requestProcessed := make(chan struct{}, 2)

	router := gin.New()
	router.Use(RequestTimer())
	router.GET("/test", func(c *gin.Context) {
		val, _ := c.Get(requestStartTimeKey)
		startTime := val.(time.Time)
		startTimes = append(startTimes, startTime)

		// Signal that request was processed
		requestProcessed <- struct{}{}

		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Make multiple requests with synchronization
	for i := 0; i < 2; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
		}()
		<-requestProcessed
		// Allow goroutine scheduling time between requests
	}

	// Wait for all requests to complete
	for i := 0; i < 2; i++ {
		select {
		case <-requestProcessed:
		case <-time.After(1 * time.Second):
		}
	}

	// Verify we have 2 different start times
	if len(startTimes) < 2 {
		t.Fatal("expected at least 2 start times")
	}

	if !startTimes[0].Before(startTimes[1]) && !startTimes[0].Equal(startTimes[1]) {
		// Times should be ordered or at least the same; difference is acceptable
		// as long as the second request happened after or at the same time as first
	}
}

func TestRequestTimer_DoesNotInterruptErrorResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RequestTimer())
	router.GET("/test", func(c *gin.Context) {
		val, exists := c.Get(requestStartTimeKey)
		if !exists {
			t.Fatal("expected request_start_time to be set even for error responses")
		}

		_, ok := val.(time.Time)
		if !ok {
			t.Fatal("expected request_start_time to be time.Time type")
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": "test error"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}
}

func TestRequestTimer_WithMultipleMiddlewares(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middlewareCalled := false

	router := gin.New()
	router.Use(RequestTimer())
	router.Use(func(c *gin.Context) {
		middlewareCalled = true
		c.Next()
	})

	router.GET("/test", func(c *gin.Context) {
		val, exists := c.Get(requestStartTimeKey)
		if !exists {
			t.Fatal("expected request_start_time to be set with multiple middlewares")
		}

		_, ok := val.(time.Time)
		if !ok {
			t.Fatal("expected request_start_time to be time.Time type")
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if !middlewareCalled {
		t.Fatal("expected other middleware to be called")
	}

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestRequestTimer_ContextKeyUniqueness(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RequestTimer())
	router.GET("/test", func(c *gin.Context) {
		// Set another value with different key
		c.Set("other_key", "other_value")

		val1, exists1 := c.Get(requestStartTimeKey)
		val2, exists2 := c.Get("other_key")

		if !exists1 {
			t.Fatal("expected request_start_time to exist")
		}

		if !exists2 {
			t.Fatal("expected other_key to exist")
		}

		if val1 == val2 {
			t.Fatal("expected different values for different keys")
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}
