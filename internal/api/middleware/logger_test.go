package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestLogger(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create middleware
	middleware := RequestLogger()

	if middleware == nil {
		t.Fatal("RequestLogger() returned nil")
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

func TestRequestLogger_WithQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middleware := RequestLogger()

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

func TestRequestLogger_ErrorStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middleware := RequestLogger()

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

func TestRequestLogger_WarningStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middleware := RequestLogger()

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

func TestRequestRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middleware := RequestRecovery()

	if middleware == nil {
		t.Fatal("RequestRecovery() returned nil")
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

func TestRequestRecovery_NoPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middleware := RequestRecovery()

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

func TestRequestMiddleware_Combined(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RequestLogger())
	router.Use(RequestRecovery())

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

func TestRequestMiddleware_Standalone(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Test middleware operates correctly as a standalone middleware
	middleware := RequestLogger()

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
