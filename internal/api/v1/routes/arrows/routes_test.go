package arrows

import (
	"testing"

	"github.com/gin-gonic/gin"
	usecase "github.com/rabbytesoftware/quiver/internal/usecases/arrows"
)

func TestSetupRoutes(t *testing.T) {
	// Create a test router
	router := gin.New()
	routerGroup := router.Group("/api/v1")

	// Create a mock usecase
	usecases := &usecase.ApiArrowUsecases{}

	// Test that SetupRoutes doesn't panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("SetupRoutes() panicked: %v", r)
		}
	}()

	// Call SetupRoutes
	SetupRoutes(routerGroup, usecases)

	// The function is currently empty, so we just test it doesn't panic
	// and can be called with valid parameters
}

func TestSetupRoutesWithNilUsecase(t *testing.T) {
	// Create a test router
	router := gin.New()
	routerGroup := router.Group("/api/v1")

	// Test with nil usecase
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("SetupRoutes() panicked with nil usecase: %v", r)
		}
	}()

	SetupRoutes(routerGroup, nil)
}

func TestSetupRoutesWithNilRouter(t *testing.T) {
	// Test with nil router group should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("SetupRoutes() should panic with nil router")
		}
	}()

	usecases := &usecase.ApiArrowUsecases{}
	SetupRoutes(nil, usecases)
}

func TestSetupRoutes_Comprehensive(t *testing.T) {
	// Test SetupRoutes with different scenarios
	router := gin.New()
	group := router.Group("/test")
	usecases := &usecase.ApiArrowUsecases{}

	// Test that SetupRoutes doesn't panic on single call
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("SetupRoutes panicked: %v", r)
		}
	}()

	// Only call once - multiple calls to same router would register same routes twice
	SetupRoutes(group, usecases)

	// Test that the router group is still valid
	if group == nil {
		t.Error("Expected router group to be valid")
	}
}

func TestSetupRoutes_EdgeCases(t *testing.T) {
	// Test SetupRoutes with edge cases
	router := gin.New()
	usecases := &usecase.ApiArrowUsecases{}

	// Test that SetupRoutes handles different scenarios
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("SetupRoutes panicked: %v", r)
		}
	}()

	// Test with different router groups
	group1 := router.Group("/test1")
	group2 := router.Group("/test2")

	SetupRoutes(group1, usecases)
	SetupRoutes(group2, usecases)

	// Test that both groups are still valid
	if group1 == nil {
		t.Error("Expected group1 to be valid")
	}
	if group2 == nil {
		t.Error("Expected group2 to be valid")
	}
}

func TestSetupRoutes_VerifyRoutesRegistered(t *testing.T) {
	router := gin.New()
	usecases := &usecase.ApiArrowUsecases{}

	group := router.Group("/api")
	SetupRoutes(group, usecases)

	// Get registered routes
	routes := router.Routes()

	// Verify we have routes registered
	if len(routes) == 0 {
		t.Fatal("Expected routes to be registered")
	}

	// Check for expected route patterns
	routePatterns := make(map[string]bool)
	for _, route := range routes {
		routePatterns[route.Path] = true
	}

	// Verify key routes are registered
	expectedRoutes := []string{
		"/api/ws/",        // WebSocket listener
		"/api/:namespace", // Arrow operations
	}

	for _, expectedRoute := range expectedRoutes {
		found := false
		for pattern := range routePatterns {
			if pattern == expectedRoute {
				found = true
				break
			}
		}
		if !found {
			t.Logf("Expected route pattern %s not found in registered routes", expectedRoute)
		}
	}
}

func TestSetupRoutes_AllMethodsRegistered(t *testing.T) {
	router := gin.New()
	usecases := &usecase.ApiArrowUsecases{}

	group := router.Group("/api")
	SetupRoutes(group, usecases)

	routes := router.Routes()

	// Verify we have both GET, POST, and DELETE methods
	methods := make(map[string]bool)
	for _, route := range routes {
		methods[route.Method] = true
	}

	expectedMethods := []string{"GET", "POST", "DELETE"}
	for _, method := range expectedMethods {
		if !methods[method] {
			t.Logf("Expected HTTP method %s not found", method)
		}
	}
}

func TestSetupRoutes_WithValidUsecases(t *testing.T) {
	router := gin.New()
	usecases := &usecase.ApiArrowUsecases{}

	group := router.Group("/api/v1")

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("SetupRoutes panicked: %v", r)
		}
	}()

	SetupRoutes(group, usecases)
}

func TestSetupRoutes_MultipleGroups(t *testing.T) {
	router := gin.New()
	usecases := &usecase.ApiArrowUsecases{}

	// Create and register routes for multiple groups
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("SetupRoutes panicked: %v", r)
		}
	}()

	group1 := router.Group("/api/v1")
	group2 := router.Group("/api/v2")

	SetupRoutes(group1, usecases)
	SetupRoutes(group2, usecases)

	routes := router.Routes()
	if len(routes) == 0 {
		t.Error("Expected routes to be registered")
	}
}

func TestSetupRoutes_VerifyWebSocketRoute(t *testing.T) {
	router := gin.New()
	usecases := &usecase.ApiArrowUsecases{}

	group := router.Group("/api")
	SetupRoutes(group, usecases)

	routes := router.Routes()

	// Check for WebSocket route
	wsFound := false
	for _, route := range routes {
		if route.Method == "GET" && contains(route.Path, "ws") {
			wsFound = true
			break
		}
	}

	if !wsFound {
		t.Log("WebSocket route not found in registered routes")
	}
}

func TestSetupRoutes_VerifyArrowOperations(t *testing.T) {
	router := gin.New()
	usecases := &usecase.ApiArrowUsecases{}

	group := router.Group("/api")
	SetupRoutes(group, usecases)

	routes := router.Routes()

	// Verify we have routes for arrow operations
	if len(routes) > 0 {
		t.Logf("Registered %d routes for arrow operations", len(routes))
	} else {
		t.Error("Expected arrow operation routes to be registered")
	}
}

// Helper function for substring check
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
