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
	usecasesInstance := usecases.NewUsecases(repos)

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
	usecasesInstance := usecases.NewUsecases(repos)

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
	usecasesInstance := usecases.NewUsecases(repos)

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
	usecasesInstance := usecases.NewUsecases(repos)

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
	usecasesInstance := usecases.NewUsecases(repos)

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
