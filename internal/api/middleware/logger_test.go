package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestWatcherLogger(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create middleware
	middleware := WatcherLogger()

	if middleware == nil {
		t.Fatal("WatcherLogger() returned nil")
	}

	// Create a test router with the middleware
	router := gin.New()
	router.Use(middleware)
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "test"})
	})

	// Test successful request (200)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestWatcherLogger_WithQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middleware := WatcherLogger()

	router := gin.New()
	router.Use(middleware)
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "test"})
	})

	// Test request with query parameters
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test?param=value&other=123", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestWatcherLogger_ErrorStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middleware := WatcherLogger()

	router := gin.New()
	router.Use(middleware)
	router.GET("/error", func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server error"})
	})

	// Test 500 error
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/error", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

func TestWatcherLogger_WarningStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middleware := WatcherLogger()

	router := gin.New()
	router.Use(middleware)
	router.GET("/notfound", func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	})

	// Test 404 warning
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/notfound", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestWatcherRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middleware := WatcherRecovery()

	if middleware == nil {
		t.Fatal("WatcherRecovery() returned nil")
	}

	router := gin.New()
	router.Use(middleware)
	router.GET("/panic", func(c *gin.Context) {
		panic("test panic")
	})

	// Test panic recovery
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/panic", nil)
	router.ServeHTTP(w, req)

	// Should recover and return 500
	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d after panic recovery, got %d", http.StatusInternalServerError, w.Code)
	}
}

func TestWatcherRecovery_NoPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middleware := WatcherRecovery()

	router := gin.New()
	router.Use(middleware)
	router.GET("/normal", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "normal"})
	})

	// Test normal operation (no panic)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/normal", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestWatcherMiddleware_Combined(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(WatcherLogger())
	router.Use(WatcherRecovery())

	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "combined middleware test"})
	})

	// Test both middlewares working together
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestWatcherMiddleware_WithNilWatcher(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Test behavior with nil watcher (should not panic since we use package-level functions)
	middleware := WatcherLogger()

	router := gin.New()
	router.Use(middleware)
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "test"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	router.ServeHTTP(w, req)

	// Should not panic and should return 200
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}
