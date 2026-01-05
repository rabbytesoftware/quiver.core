package arrows

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	arrowsUsecases "github.com/rabbytesoftware/quiver/internal/api/v1/usecases/arrows"
	"github.com/rabbytesoftware/quiver/internal/infrastructure"
	"github.com/rabbytesoftware/quiver/internal/repositories"
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
	router.POST("/api/v1/arrow/:namespace", h.AddArrow)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/arrow/QUID:AUID", nil)
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
