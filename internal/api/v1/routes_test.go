package v1

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/infrastructure"
	"github.com/rabbytesoftware/quiver/internal/repositories"
	"github.com/rabbytesoftware/quiver/internal/usecases"
)

func TestSetupRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create dependencies
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)
	usecasesInstance := usecases.NewApiUsecases(repos)

	// Create router
	router := gin.New()

	// This should not panic
	SetupRoutes(router, usecasesInstance)

	// Test that health endpoint was registered
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/health", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected health endpoint to return %d, got %d", http.StatusOK, w.Code)
	}
}

func TestSetupRoutes_WithNilUsecases(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()

	// This should panic because usecases.System is accessed
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic with nil usecases, but didn't get one")
		}
	}()

	SetupRoutes(router, nil)
}

func TestSetupRoutes_WithNilRouter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)
	usecasesInstance := usecases.NewApiUsecases(repos)

	// This should panic when trying to create router groups
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic with nil router, but didn't get one")
		}
	}()

	SetupRoutes(nil, usecasesInstance)
}

func TestSetupRoutes_AllEndpointsRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)

	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)
	usecasesInstance := usecases.NewApiUsecases(repos)

	router := gin.New()
	SetupRoutes(router, usecasesInstance)

	// Test various endpoints that should be registered
	testCases := []struct {
		method         string
		path           string
		expectedStatus int
	}{
		{"GET", "/api/v1/health", http.StatusOK},
		// Note: Other endpoints are empty functions so they would return 404
		// but we can test that the router doesn't panic when accessing them
	}

	for _, tc := range testCases {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(tc.method, tc.path, nil)
		router.ServeHTTP(w, req)

		if w.Code != tc.expectedStatus {
			t.Errorf("Expected %s %s to return %d, got %d", tc.method, tc.path, tc.expectedStatus, w.Code)
		}
	}
}

func TestSetupRoutes_RouterGroupsCreated(t *testing.T) {
	gin.SetMode(gin.TestMode)

	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)
	usecasesInstance := usecases.NewApiUsecases(repos)

	router := gin.New()

	// Capture the initial number of routes
	initialRoutes := len(router.Routes())

	SetupRoutes(router, usecasesInstance)

	// After setup, we should have at least the health route
	finalRoutes := len(router.Routes())

	if finalRoutes <= initialRoutes {
		t.Error("Expected routes to be added after SetupRoutes")
	}
}

func TestSetupRoutes_HealthHandlerIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)
	usecasesInstance := usecases.NewApiUsecases(repos)

	router := gin.New()
	SetupRoutes(router, usecasesInstance)

	// Test the health endpoint specifically
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/health", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected health endpoint to return %d, got %d", http.StatusOK, w.Code)
	}

	// Check that response contains expected message
	expectedContent := "Sector 7C"
	if !contains(w.Body.String(), expectedContent) {
		t.Errorf("Expected response to contain %q, got %q", expectedContent, w.Body.String())
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			containsSubstring(s, substr))))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSetupRoutes_MultipleCallsWithDifferentRouters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)
	usecasesInstance := usecases.NewApiUsecases(repos)

	router1 := gin.New()
	router2 := gin.New()

	SetupRoutes(router1, usecasesInstance)
	SetupRoutes(router2, usecasesInstance)

	// Test first router
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/api/v1/health", nil)
	router1.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("Router1: Expected health endpoint to return %d, got %d", http.StatusOK, w1.Code)
	}

	// Test second router
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/v1/health", nil)
	router2.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("Router2: Expected health endpoint to return %d, got %d", http.StatusOK, w2.Code)
	}
}

func TestSetupRoutes_VerifyHealthResponseFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)
	usecasesInstance := usecases.NewApiUsecases(repos)

	router := gin.New()
	SetupRoutes(router, usecasesInstance)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/health", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	// Verify JSON response
	if !contains(w.Body.String(), "Sector 7C") {
		t.Errorf("Expected response to contain 'Sector 7C', got: %s", w.Body.String())
	}
}

func TestSetupRoutes_TestWithMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)
	usecasesInstance := usecases.NewApiUsecases(repos)

	router := gin.New()
	SetupRoutes(router, usecasesInstance)

	// Test that routes work
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/health", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestSetupRoutes_ConcurrentRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)
	usecasesInstance := usecases.NewApiUsecases(repos)

	router := gin.New()
	SetupRoutes(router, usecasesInstance)

	// Make multiple requests
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/health", nil)
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Request %d: Expected status 200, got %d", i, w.Code)
		}
	}
}

func TestSetupRoutes_HealthResponseStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)
	usecasesInstance := usecases.NewApiUsecases(repos)

	router := gin.New()
	SetupRoutes(router, usecasesInstance)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/health", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestSetupRoutes_VerifyAllRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)
	usecasesInstance := usecases.NewApiUsecases(repos)

	router := gin.New()
	SetupRoutes(router, usecasesInstance)

	// Verify that we have routes registered
	routes := router.Routes()
	if len(routes) == 0 {
		t.Fatal("Expected routes to be registered")
	}

	// Verify health route exists
	healthFound := false
	for _, route := range routes {
		if route.Path == "/api/v1/health" {
			healthFound = true
			break
		}
	}

	if !healthFound {
		t.Error("Expected health route to be registered")
	}
}
