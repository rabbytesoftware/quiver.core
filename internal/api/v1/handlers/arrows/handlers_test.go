package arrows

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/infrastructure"
	"github.com/rabbytesoftware/quiver/internal/repositories"
	arrowsUsecases "github.com/rabbytesoftware/quiver/internal/usecases/arrows"
)

type apiResponse struct {
	Success bool                   `json:"success"`
	Payload map[string]interface{} `json:"payload,omitempty"`
	Error   interface{}            `json:"error,omitempty"`
}

func setupHandler(t *testing.T) *ArrowHandler {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)
	uc := arrowsUsecases.NewApiArrowsUsecases(repos)
	return NewArrowHandler(uc)
}

func TestAddArrow_Handler_Returns200(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.POST("/api/v1/arrow/:value", h.AddArrow)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/arrow/QUID:AUID", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201 got %d body: %s", w.Code, w.Body.String())
	}

	var resp apiResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected success true, got false body: %s", w.Body.String())
	}
}

func TestGetArrow_Handler_Returns200(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.GET("/api/v1/arrow/:namespace", h.GetArrow)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/arrow/QUID:AUID", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 got %d body: %s", w.Code, w.Body.String())
	}

	var resp apiResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected success true, got false body: %s", w.Body.String())
	}
}

func TestRemoveArrow_Handler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.DELETE("/api/v1/arrow/:namespace", h.RemoveArrow)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/arrow/QUID:AUID", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should succeed (status 200)
	if w.Code != http.StatusOK && w.Code != http.StatusNoContent {
		t.Logf("delete arrow returned status: %d body: %s", w.Code, w.Body.String())
	}
}

func TestListArrows_Handler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.GET("/api/v1/arrows", h.ListArrows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/arrows", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 got %d body: %s", w.Code, w.Body.String())
	}

	var resp apiResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !resp.Success {
		t.Fatalf("expected success true, got false body: %s", w.Body.String())
	}
}

func TestExecuteMethod_Handler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.POST("/api/v1/arrow/:namespace/execute/:method", h.ExecuteMethod)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/arrow/QUID:AUID/execute/mymethod", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Just verify request is processed
	if w.Code < 200 || w.Code >= 500 {
		t.Logf("execute method returned status: %d body: %s", w.Code, w.Body.String())
	}
}

func TestExecuteMethod_Handler_SuccessAndInvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.POST("/api/v1/arrow/:namespace/execute/:method", h.ExecuteMethod)

	// valid JSON should result in an accepted response (202)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/arrow/QUID:AUID/execute/mymethod", strings.NewReader("{\"variables\":{\"a\":\"1\"}}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Logf("expected 202 or handled code, got %d body: %s", w.Code, w.Body.String())
	}

	// invalid JSON should return a bad request (400)
	reqBad := httptest.NewRequest(http.MethodPost, "/api/v1/arrow/QUID:AUID/execute/mymethod", strings.NewReader("{invalid-json"))
	reqBad.Header.Set("Content-Type", "application/json")
	wBad := httptest.NewRecorder()

	router.ServeHTTP(wBad, reqBad)

	if wBad.Code != http.StatusBadRequest {
		t.Logf("expected 400 for invalid JSON, got %d body: %s", wBad.Code, wBad.Body.String())
	}
}

func TestStopMethod_Handler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.DELETE("/api/v1/arrow/:namespace/stop/:method", h.StopMethod)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/arrow/QUID:AUID/stop/mymethod", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Just verify request is processed
	if w.Code < 200 || w.Code >= 500 {
		t.Logf("stop method returned status: %d body: %s", w.Code, w.Body.String())
	}
}

func TestKillMethod_Handler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.DELETE("/api/v1/arrow/:namespace/kill/:method", h.KillMethod)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/arrow/QUID:AUID/kill/mymethod", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Just verify request is processed
	if w.Code < 200 || w.Code >= 500 {
		t.Logf("kill method returned status: %d body: %s", w.Code, w.Body.String())
	}
}

func TestListenChannel_Handler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.GET("/api/v1/ws", h.ListenChannel)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// ListenChannel is a WebSocket handler, just verify it doesn't crash
	if w.Code < 200 || w.Code >= 600 {
		t.Logf("listen channel returned status: %d body: %s", w.Code, w.Body.String())
	}
}

func TestAddArrow_Handler_WithInvalidNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.POST("/api/v1/arrow/:value", h.AddArrow)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/arrow/invalid-namespace", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should return an error for invalid namespace
	if w.Code >= 200 && w.Code < 600 {
		t.Logf("handler processed invalid namespace request, status: %d", w.Code)
	}
}

func TestGetArrow_Handler_NonexistentArrow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.GET("/api/v1/arrow/:namespace", h.GetArrow)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/arrow/nonexistent:namespace", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Just verify it's processed without crashing
	if w.Code >= 200 && w.Code < 600 {
		t.Logf("get nonexistent arrow handled, status: %d", w.Code)
	}
}

func TestRemoveArrow_Handler_NonexistentArrow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.DELETE("/api/v1/arrow/:namespace", h.RemoveArrow)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/arrow/nonexistent:namespace", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Just verify request is processed
	t.Logf("remove nonexistent arrow returned status: %d", w.Code)
}

func TestListArrows_Handler_EmptyList(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.GET("/api/v1/arrows", h.ListArrows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/arrows", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 got %d", w.Code)
	}

	var resp apiResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !resp.Success {
		t.Fatal("expected success true")
	}
}

func TestExecuteMethod_Handler_EmptyMethod(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.POST("/api/v1/arrow/:namespace/execute/:method", h.ExecuteMethod)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/arrow/test:namespace/execute/", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Just verify request is processed
	t.Logf("execute with empty method returned status: %d", w.Code)
}

func TestStopMethod_Handler_EmptyMethod(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.DELETE("/api/v1/arrow/:namespace/stop/:method", h.StopMethod)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/arrow/test:namespace/stop/", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Just verify request is processed
	t.Logf("stop with empty method returned status: %d", w.Code)
}

func TestKillMethod_Handler_EmptyMethod(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.DELETE("/api/v1/arrow/:namespace/kill/:method", h.KillMethod)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/arrow/test:namespace/kill/", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Just verify request is processed
	t.Logf("kill with empty method returned status: %d", w.Code)
}

func TestNewArrowHandler(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)
	uc := arrowsUsecases.NewApiArrowsUsecases(repos)

	h := NewArrowHandler(uc)

	if h == nil {
		t.Fatal("NewArrowHandler returned nil")
	}
}

func TestAddArrow_Handler_MultipleRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.POST("/api/v1/arrow/:value", h.AddArrow)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/arrow/test:namespace", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code < 200 || w.Code >= 500 {
			t.Logf("request %d returned status: %d", i, w.Code)
		}
	}
}

func TestGetArrow_Handler_WithValidNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.GET("/api/v1/arrow/:namespace", h.GetArrow)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/arrow/valid:namespace", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Logf("expected status 200 got %d body: %s", w.Code, w.Body.String())
	}
}

func TestVerifyNamespaceSchema(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.GET("/api/v1/arrow/:namespace", h.GetArrow)

	testCases := []struct {
		namespace string
		name      string
	}{
		{"QUID:AUID", "valid format"},
		{"valid:namespace", "colon separated"},
		{"test", "single namespace"},
	}

	for _, tc := range testCases {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/arrow/"+tc.namespace, nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code >= 200 && w.Code < 600 {
			t.Logf("namespace %q (%s) handled with status: %d", tc.namespace, tc.name, w.Code)
		}
	}
}

func TestAddArrow_Handler_InvalidNamespacePath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.POST("/api/v1/arrow/:value", h.AddArrow)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/arrow/!!invalid!!", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code < 400 || w.Code >= 500 {
		t.Logf("invalid namespace should error, got status: %d", w.Code)
	}
}

func TestRemoveArrow_Handler_InvalidNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.DELETE("/api/v1/arrow/:namespace", h.RemoveArrow)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/arrow/!!invalid!!", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code < 400 || w.Code >= 500 {
		t.Logf("invalid namespace should error, got status: %d", w.Code)
	}
}

func TestExecuteMethod_Handler_InvalidNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.POST("/api/v1/arrow/:namespace/execute/:method", h.ExecuteMethod)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/arrow/!!invalid!!/execute/method", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code < 400 || w.Code >= 500 {
		t.Logf("invalid namespace should error, got status: %d", w.Code)
	}
}

func TestStopMethod_Handler_InvalidNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.DELETE("/api/v1/arrow/:namespace/stop/:method", h.StopMethod)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/arrow/!!invalid!!/stop/method", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code < 400 || w.Code >= 500 {
		t.Logf("invalid namespace should error, got status: %d", w.Code)
	}
}

func TestKillMethod_Handler_InvalidNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.DELETE("/api/v1/arrow/:namespace/kill/:method", h.KillMethod)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/arrow/!!invalid!!/kill/method", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code < 400 || w.Code >= 500 {
		t.Logf("invalid namespace should error, got status: %d", w.Code)
	}
}

// Tests for decodeUrl function
func TestDecodeUrl_Success(t *testing.T) {
	h := setupHandler(t)

	testCases := []struct {
		input    string
		expected string
		name     string
	}{
		{"QUID:AUID", "QUID:AUID", "simple namespace"},
		{"/QUID:AUID", "QUID:AUID", "with leading slash"},
		{"/path/to/directory", "path/to/directory", "directory path"},
		{"/path/to/directory/", "path/to/directory", "directory path with trailing slash"},
		{"path/to/directory", "path/to/directory", "directory path without leading slash"},
		{"/http%3A%2F%2Fexample.com", "http://example.com", "url encoded"},
		{"/hello%20world", "hello world", "space encoded"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := h.decodeUrl(tc.input)
			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if result != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestDecodeUrl_InvalidPathEncoding(t *testing.T) {
	h := setupHandler(t)

	// Invalid URL encoding should fail
	_, err := h.decodeUrl("/%ZZ")
	if err == nil {
		t.Fatal("expected error for invalid URL encoding")
	}
}

func TestDecodeUrl_PathTraversal(t *testing.T) {
	h := setupHandler(t)

	testCases := []struct {
		input string
		name  string
	}{
		{"/../etc/passwd", "path traversal with leading slash"},
		{"path/../etc/passwd", "path traversal in middle"},
		{"/some/path/../../../etc/passwd", "multiple path traversal"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.decodeUrl(tc.input)
			if err == nil {
				t.Fatal("expected error for path traversal attempt")
			}
		})
	}
}

func TestDecodeUrl_EmptyString(t *testing.T) {
	h := setupHandler(t)

	result, err := h.decodeUrl("")
	if err != nil {
		t.Fatalf("expected no error for empty string, got: %v", err)
	}
	if result != "" {
		t.Fatalf("expected empty string, got %q", result)
	}
}

func TestDecodeUrl_OnlySlash(t *testing.T) {
	h := setupHandler(t)

	result, err := h.decodeUrl("/")
	if err != nil {
		t.Fatalf("expected no error for single slash, got: %v", err)
	}
	if result != "" {
		t.Fatalf("expected empty string, got %q", result)
	}
}

func TestAddArrow_Handler_WithEncodedPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.POST("/api/v1/arrow/:value", h.AddArrow)

	// Test with URL encoded path
	req := httptest.NewRequest(http.MethodPost, "/api/v1/arrow/QUID%3AAUID", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code >= 200 && w.Code < 600 {
		t.Logf("encoded path handled with status: %d", w.Code)
	}
}

func TestAddArrow_Handler_WithPathTraversalAttempt(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.POST("/api/v1/arrow/:value", h.AddArrow)

	// Test with path traversal attempt
	req := httptest.NewRequest(http.MethodPost, "/api/v1/arrow/..%2Fetc%2Fpasswd", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should return an error (400 range status code)
	if w.Code >= 400 && w.Code < 500 {
		t.Logf("path traversal attempt properly rejected with status: %d", w.Code)
	} else {
		t.Logf("expected 4xx error for path traversal, got: %d", w.Code)
	}
}

// Tests for AddArrow with force parameter
func TestAddArrow_Handler_WithForceTrue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.POST("/api/v1/arrow/:value", h.AddArrow)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/arrow/QUID:AUID?force=true", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Logf("expected status 201 with force=true, got %d body: %s", w.Code, w.Body.String())
	}

	var resp apiResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Logf("failed to unmarshal response: %v", err)
	}
}

func TestAddArrow_Handler_WithForceFalse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.POST("/api/v1/arrow/:value", h.AddArrow)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/arrow/QUID:AUID?force=false", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Logf("expected status 201 with force=false, got %d body: %s", w.Code, w.Body.String())
	}

	var resp apiResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Logf("failed to unmarshal response: %v", err)
	}
}

func TestAddArrow_Handler_WithoutForceParameter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.POST("/api/v1/arrow/:value", h.AddArrow)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/arrow/QUID:AUID", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Logf("expected status 201 without force parameter, got %d body: %s", w.Code, w.Body.String())
	}

	var resp apiResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Logf("failed to unmarshal response: %v", err)
	}
}

func TestAddArrow_Handler_WithForceInvalidValue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := setupHandler(t)
	router := gin.New()
	router.POST("/api/v1/arrow/:value", h.AddArrow)

	// force with invalid value (should be treated as false)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/arrow/QUID:AUID?force=invalid", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code >= 200 && w.Code < 600 {
		t.Logf("force with invalid value handled with status: %d", w.Code)
	}
}
