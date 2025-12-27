package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	usecase "github.com/rabbytesoftware/quiver/internal/usecases/system"
)

func TestNewHealthHandler(t *testing.T) {
	usecases := &usecase.SystemUsecase{}
	handler := NewHealthHandler(usecases)

	if handler == nil {
		t.Fatal("NewHealthHandler() returned nil")
	}

	if handler.usecases != usecases {
		t.Error("NewHealthHandler() did not set usecases correctly")
	}
}

func TestHealthHandler_SetupRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	usecases := &usecase.SystemUsecase{}
	handler := NewHealthHandler(usecases)

	router := gin.New()
	group := router.Group("/api/v1")

	// This should not panic
	handler.SetupRoutes(group)

	// Verify the route was registered by making a request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/health", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestHealthHandler_Handler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	usecases := &usecase.SystemUsecase{}
	handler := NewHealthHandler(usecases)

	// Create a test router
	router := gin.New()
	router.GET("/health", handler.Handler())

	// Create a test request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)

	// Perform the request
	router.ServeHTTP(w, req)

	// Check the response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Check the response body
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	expectedMessage := "Sector 7C"
	if response["message"] != expectedMessage {
		t.Errorf("Expected message %q, got %q", expectedMessage, response["message"])
	}
}

func TestHealthHandler_HandlerWithNilUsecases(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Test with nil usecases (should still work since handler doesn't use them)
	handler := NewHealthHandler(nil)

	router := gin.New()
	router.GET("/health", handler.Handler())

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestHealthHandler_MultipleRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	usecases := &usecase.SystemUsecase{}
	handler := NewHealthHandler(usecases)

	router := gin.New()
	router.GET("/health", handler.Handler())

	// Test multiple requests to ensure consistency
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/health", nil)

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Request %d: Expected status %d, got %d", i+1, http.StatusOK, w.Code)
		}

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		if err != nil {
			t.Fatalf("Request %d: Failed to unmarshal response: %v", i+1, err)
		}

		if response["message"] != "Sector 7C" {
			t.Errorf("Request %d: Expected message 'Sector 7C', got %q", i+1, response["message"])
		}
	}
}
